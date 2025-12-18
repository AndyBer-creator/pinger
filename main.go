package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fatih/color"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

var (
	count    = flag.Int("c", 0, "stop after count packets (0 = infinite)")
	interval = flag.Duration("i", time.Second, "interval between packets (min 1ms)")
	output   = flag.String("o", "", "write statistics to JSON file")
	size     = flag.Int("s", 56, "ICMP data size (default 56, max 1472)")
	mtuTest  = flag.Bool("mtu-test", false, "auto discover MTU (jumbo frames support)")
	version  = flag.Bool("V", false, "show version")
	verbose  = flag.Bool("v", false, "verbose statistics")
	liveFlag = flag.Bool("live", false, "live statistics every 10s")

	ttl       = flag.Int("t", 64, "IP TTL (1-255, default 64)")
	traceMode = flag.Bool("trace", false, "traceroute-like mode (increment TTL from 1 to -t)")

	mu          sync.RWMutex
	PacketsSent int
	PacketsRecv int
	RTTStats    map[int]time.Duration

	green   = color.New(color.FgGreen).SprintFunc()
	yellow  = color.New(color.FgYellow).SprintFunc()
	red     = color.New(color.FgRed).SprintFunc()
	blue    = color.New(color.FgBlue).SprintFunc()
	magenta = color.New(color.FgMagenta).SprintFunc()
	cyan    = color.New(color.FgCyan).SprintFunc()
)

type PingStats struct {
	PacketsSent  int               `json:"packets_sent"`
	PacketsRecv  int               `json:"packets_received"`
	PacketLoss   float64           `json:"packet_loss_percent"`
	MinRTT       time.Duration     `json:"min_rtt"`
	AvgRTT       time.Duration     `json:"avg_rtt"`
	MaxRTT       time.Duration     `json:"max_rtt"`
	Measurements []PingMeasurement `json:"measurements"`
}

type PingMeasurement struct {
	Seq int           `json:"seq"`
	RTT time.Duration `json:"rtt"`
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options] <host>\n\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintln(flag.CommandLine.Output(), "\nExamples:")
		fmt.Fprintf(flag.CommandLine.Output(), "  %s -V\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s -c 10 -i 5ms 192.168.1.1\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s -c 100 -i 1ms -live -v -o stats.json 8.8.8.8\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s --trace -t 30 8.8.8.8\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s -t 32 -s 1472 192.168.1.1\n", os.Args[0])
	}

	flag.Parse()

	if *version {
		fmt.Printf("pinger v1.2.1 (built %s)\n", time.Now().Format("2006-01-02"))
		fmt.Printf("Backend developer tools - MTU/Jumbo ping + traceroute utility\n")
		os.Exit(0)
	}

	if len(flag.Args()) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	host := flag.Arg(0)

	if *interval < time.Millisecond {
		log.Fatal("interval must be >= 1ms")
	}
	if *size < 0 || *size > 1472 {
		log.Fatal("size must be 0-1472 bytes")
	}
	if *ttl < 1 || *ttl > 255 {
		log.Fatal("TTL must be 1-255")
	}

	dst, err := net.ResolveIPAddr("ip", host)
	if err != nil {
		log.Fatalf("resolve error: %v", err)
	}

	c, err := pingConn(dst)
	if err != nil {
		log.Fatalf("listen error: %v (Windows: no admin needed; Linux: needs root/CAP_NET_RAW)", err)
	}
	defer c.Close()

	id := os.Getpid() & 0xffff

	// ✅ Правильный способ получить IPv4 PacketConn
	var pconn4 *ipv4.PacketConn
	if icmpConn, ok := c.(*icmp.PacketConn); ok {
		if p4 := icmpConn.IPv4PacketConn(); p4 != nil {
			pconn4 = p4
			if !*traceMode {
				pconn4.SetTTL(*ttl)
			}
		}
	}

	mu.Lock()
	PacketsSent = 0
	PacketsRecv = 0
	RTTStats = make(map[int]time.Duration)
	mu.Unlock()

	if *mtuTest {
		discoverMTU(dst)
	}

	if *liveFlag && !*traceMode {
		liveTicker := time.NewTicker(10 * time.Second)
		go func() {
			defer liveTicker.Stop()
			for range liveTicker.C {
				printLiveStats()
			}
		}()
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sig
		printStats()
		if *output != "" {
			writeJSONStats(*output)
		}
		os.Exit(0)
	}()

	seq := 1
	if *traceMode {
		runTraceroute(c, pconn4, dst, id, &seq)
	} else {
		runPing(c, dst, id, &seq)
	}

	printStats()
	if *output != "" {
		writeJSONStats(*output)
	}
}

func runPing(c net.PacketConn, dst *net.IPAddr, id int, seq *int) {
	for *count == 0 || PacketsSent < *count {
		sendAndRecvPing(c, dst, id, *seq) // ✅ *seq -> int
		*seq++
		time.Sleep(*interval)
	}
}

func runTraceroute(c net.PacketConn, p4 *ipv4.PacketConn, dst *net.IPAddr, id int, seq *int) {
	fmt.Printf("%s traceroute to %s (%s), %d hops max\n\n",
		magenta("TRACE"), dst.String(), blue(dst.IP.String()), *ttl)

	maxHops := *ttl
	for hop := 1; hop <= maxHops; hop++ {
		if p4 != nil {
			p4.SetTTL(hop)
		}

		fmt.Printf("%2d ", hop)
		sendAndRecvPing(c, dst, id, *seq) //  *seq -> int
		*seq++

		// Если получили ответ от цели - выходим
		mu.RLock()
		if _, ok := RTTStats[*seq-1]; ok {
			fmt.Print(green(" DEST!"))
			mu.RUnlock()
			break
		}
		mu.RUnlock()

		fmt.Println()
		time.Sleep(50 * time.Millisecond)
	}
	fmt.Println()
}

func sendAndRecvPing(c net.PacketConn, dst *net.IPAddr, id, seq int) {
	msg, err := icmpMsg(dst.IP, id, seq)
	if err != nil {
		log.Printf("marshal error: %v", err)
		return
	}
	wb, err := msg.Marshal(nil)
	if err != nil {
		log.Printf("marshal error: %v", err)
		return
	}

	deadline := time.Now().Add(*interval * 10)
	c.SetReadDeadline(deadline)

	start := time.Now()
	mu.Lock()
	PacketsSent++
	mu.Unlock()

	if _, err := c.WriteTo(wb, dst); err != nil {
		fmt.Printf("%s icmp_seq=%d\n", red("send error"), seq)
		return
	}

	recvCount := 0
	maxRecvAttempts := 5
	for recvCount < maxRecvAttempts {
		rb := make([]byte, 1500)
		n, peer, err := c.ReadFrom(rb)
		if err, ok := err.(net.Error); ok && err.Timeout() {
			break
		}
		if err != nil {
			break
		}

		rtt := time.Since(start)
		if rtt < 10*time.Microsecond {
			continue
		}

		rm, err := icmp.ParseMessage(proto(dst.IP), rb[:n])
		if err != nil {
			continue
		}

		//  ICMP TimeExceeded для traceroute (без неиспользуемой переменной)
		if _, ok := rm.Body.(*icmp.TimeExceeded); ok {
			fmt.Printf("%s %s ", cyan(peer.String()), yellow(rtt.Round(time.Millisecond)))
			return
		}

		// Echo Reply
		if echo, ok := rm.Body.(*icmp.Echo); ok && echo.ID == id && echo.Seq == seq {
			mu.Lock()
			PacketsRecv++
			RTTStats[seq] = rtt
			mu.Unlock()

			rttRounded := rtt.Round(time.Microsecond)
			if !*traceMode {
				if rttRounded < 1*time.Millisecond {
					fmt.Printf("%d bytes %s icmp_seq=%d %s\n",
						len(echo.Data), blue(peer.String()), seq, green(rttRounded))
				} else if rttRounded < 10*time.Millisecond {
					fmt.Printf("%d bytes %s icmp_seq=%d %s\n",
						len(echo.Data), blue(peer.String()), seq, yellow(rttRounded))
				} else {
					fmt.Printf("%d bytes %s icmp_seq=%d %s\n",
						len(echo.Data), blue(peer.String()), seq, red(rttRounded))
				}
			} else {
				fmt.Printf("%s %s ", green(peer.String()), green(rtt.Round(time.Millisecond)))
			}
			return
		}
		recvCount++
	}

	if !*traceMode {
		fmt.Printf("%s\n", red(fmt.Sprintf("Request timeout for icmp_seq=%d", seq)))
	} else {
		fmt.Printf("%s\n", red("*"))
	}
}

// Все остальные функции без изменений...
func printStats() {
	mu.RLock()
	sent, recv := PacketsSent, PacketsRecv
	rtts := make(map[int]time.Duration)
	for k, v := range RTTStats {
		rtts[k] = v
	}
	mu.RUnlock()

	if sent == 0 {
		return
	}

	loss := float64(sent-recv) / float64(sent) * 100

	fmt.Printf("\n%s\n", magenta("--- ping statistics ---"))
	fmt.Printf("%s transmitted, %s received, %s packet loss\n",
		blue(strconv.Itoa(sent)),
		blue(strconv.Itoa(recv)),
		magenta(fmt.Sprintf("%.1f%%", loss)))

	if recv == 0 || len(rtts) == 0 {
		return
	}

	var total, min, max time.Duration
	validRTT := 0
	rttValues := make([]time.Duration, 0, len(rtts))

	for _, rtt := range rtts {
		if rtt > 0 {
			total += rtt
			rttValues = append(rttValues, rtt)
			if validRTT == 0 || rtt < min {
				min = rtt
			}
			if validRTT == 0 || rtt > max {
				max = rtt
			}
			validRTT++
		}
	}

	if validRTT == 0 {
		return
	}

	avg := total / time.Duration(validRTT)

	minRounded := min.Round(time.Microsecond)
	avgRounded := avg.Round(time.Microsecond)
	maxRounded := max.Round(time.Millisecond)

	fmt.Printf("round-trip min/avg/max = %s/%s/%s\n",
		green(minRounded),
		yellow(avgRounded),
		red(maxRounded))

	if *verbose {
		jitter := calculateJitter(rttValues)
		bandwidth := calculateBandwidth(sent, *size, *interval)

		fmt.Printf("%s Jitter: %s\n",
			yellow(" jitter"),
			yellow(jitter.Round(time.Microsecond)))
		fmt.Printf("%s Bandwidth: %.1f Mbps\n",
			blue(" bandwidth"),
			bandwidth)
		fmt.Printf("%s Frame size: %d bytes (ICMP data: %d)\n",
			magenta(" frame"),
			*size+42, *size)
	}
}

func printLiveStats() {
	mu.RLock()
	sent, recv := PacketsSent, PacketsRecv
	rtts := make(map[int]time.Duration)
	for k, v := range RTTStats {
		rtts[k] = v
	}
	mu.RUnlock()

	if sent == 0 {
		return
	}
	loss := float64(sent-recv) / float64(sent) * 100

	valid := 0
	var avg time.Duration
	for _, rtt := range rtts {
		if rtt > 0 {
			avg += rtt
			valid++
		}
	}
	if valid > 0 {
		avg /= time.Duration(valid)
	}

	progress := ""
	if *count > 0 {
		pct := float64(sent) / float64(*count) * 100
		bars := int(pct / 5)
		if bars > 20 {
			bars = 20
		}
		progress = fmt.Sprintf(" [%s%s] %.0f%%",
			strings.Repeat("█", bars),
			strings.Repeat("░", 20-bars), pct)
	}

	fmt.Printf("\r%s %d/%d pkts%s loss:%.1f%% rtt:%s",
		blue("LIVE"), sent, *count, progress,
		loss, yellow(avg.Round(time.Microsecond)))
}

type statsResult struct {
	min, avg, max time.Duration
}

func calculateStats(rtts map[int]time.Duration) statsResult {
	var total, min, max time.Duration
	count := 0
	for _, rtt := range rtts {
		if rtt > 0 {
			total += rtt
			if count == 0 || rtt < min {
				min = rtt
			}
			if count == 0 || rtt > max {
				max = rtt
			}
			count++
		}
	}
	avg := time.Duration(0)
	if count > 0 {
		avg = total / time.Duration(count)
	}
	return statsResult{min: min, avg: avg, max: max}
}

func writeJSONStats(filename string) {
	mu.RLock()
	sent, recv := PacketsSent, PacketsRecv
	rtts := make(map[int]time.Duration)
	for k, v := range RTTStats {
		rtts[k] = v
	}
	mu.RUnlock()

	stats := calculateStats(rtts)

	measurements := make([]PingMeasurement, 0, len(rtts))
	for seq, rtt := range rtts {
		if rtt > 0 {
			measurements = append(measurements, PingMeasurement{
				Seq: seq,
				RTT: rtt,
			})
		}
	}

	result := PingStats{
		PacketsSent:  sent,
		PacketsRecv:  recv,
		PacketLoss:   float64(sent-recv) / float64(sent) * 100,
		MinRTT:       stats.min,
		AvgRTT:       stats.avg,
		MaxRTT:       stats.max,
		Measurements: measurements,
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Printf("JSON marshal error: %v", err)
		return
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		log.Printf("write file error: %v", err)
		return
	}
	fmt.Printf("Statistics saved to %s\n", filename)
}

func pingConn(dst *net.IPAddr) (net.PacketConn, error) {
	switch runtime.GOOS {
	case "windows", "darwin", "linux":
		if dst.IP.To4() != nil {
			return icmp.ListenPacket("ip4:icmp", "0.0.0.0")
		}
		return icmp.ListenPacket("ip6:ipv6-icmp", "::")
	default:
		return nil, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func icmpMsg(ip net.IP, id, seq int) (icmp.Message, error) {
	data := make([]byte, *size)
	for i := range data {
		data[i] = byte('A' + i%26)
	}

	if ip.To4() != nil {
		return icmp.Message{
			Type: ipv4.ICMPTypeEcho,
			Code: 0,
			Body: &icmp.Echo{
				ID:   id,
				Seq:  seq,
				Data: data,
			},
		}, nil
	}
	return icmp.Message{
		Type: ipv6.ICMPTypeEchoRequest,
		Code: 0,
		Body: &icmp.Echo{
			ID:   id,
			Seq:  seq,
			Data: data,
		},
	}, nil
}

func proto(ip net.IP) int {
	if ip.To4() != nil {
		return ipv4.ICMPTypeEchoReply.Protocol()
	}
	return ipv6.ICMPTypeEchoReply.Protocol()
}

func discoverMTU(dst *net.IPAddr) {
	fmt.Printf("%s Testing MTU...\n", magenta("MTU"))

	sizes := []int{1500, 9000, 12000}
	var maxSuccess int

	for _, testSize := range sizes {
		testDataSize := testSize - 28
		if testDataSize > 1472 {
			continue
		}

		oldSize := *size
		*size = testDataSize

		conn, err := pingConn(dst)
		if err != nil {
			*size = oldSize
			continue
		}

		success := 0
		id := os.Getpid() & 0xffff
		for i := 0; i < 3; i++ {
			msg, err := icmpMsg(dst.IP, id, 1)
			if err != nil {
				break
			}
			wb, err := msg.Marshal(nil)
			if err != nil {
				break
			}
			if _, err := conn.WriteTo(wb, dst); err == nil {
				success++
			}
			time.Sleep(100 * time.Millisecond)
		}
		conn.Close()
		*size = oldSize

		if success == 3 {
			maxSuccess = testSize
			fmt.Printf("  MTU %d OK\n", testSize)
		} else {
			fmt.Printf("  MTU %d FAILED (%d/3)\n", testSize, success)
			break
		}
	}

	if maxSuccess > 0 {
		*size = maxSuccess - 28
		fmt.Printf("%s Max MTU: %d bytes (size set to %d)\n\n", blue("✓"), maxSuccess, *size)
	} else {
		fmt.Printf("%s No MTU discovered, using default\n\n", yellow("!"))
	}
}

func calculateJitter(rtts []time.Duration) time.Duration {
	if len(rtts) < 2 {
		return 0
	}

	var totalDiff time.Duration
	for i := 1; i < len(rtts); i++ {
		diff := rtts[i] - rtts[i-1]
		if diff < 0 {
			diff = -diff
		}
		totalDiff += diff
	}
	return totalDiff / time.Duration(len(rtts)-1)
}

func calculateBandwidth(sent int, size int, interval time.Duration) float64 {
	totalBytes := float64(size * sent)
	totalTime := interval.Seconds() * float64(sent)
	bytesPerSec := totalBytes / totalTime
	return bytesPerSec * 8 / 1e6
}
