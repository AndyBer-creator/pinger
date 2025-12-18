**Универсальная ICMP утилита** для сетевой диагностики.  
*ping + traceroute + MTU discovery* в одном бинарнике с цветным выводом, JSON-логами и live-статистикой.  
Написана на **Go** для собственных нужд backend разработчика.

[![Go](https://img.shields.io/badge/Go-1.18%2B-blue?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Ping](https://img.shields.io/badge/Pinger-v1.2.1-brightgreen)](https://github.com/AndyBer-creator/pinger)

## ✨ **Возможности**

| Функция | Описание | Пример |
|---------|----------|---------|
| **ICMP Ping** | Классический ping с кастомным размером | `pinger -c 100 8.8.8.8` |
| **Traceroute** | Построение маршрута (TTL 1→max) | `pinger --trace -t 30 8.8.8.8` |
| **Jumbo/MTU** | Тест больших пакетов (до 9K+) | `pinger -s 1472 --mtu-test` |
| **Live stats** | Онлайн мониторинг loss/RTT | `pinger -live -v` |
| **JSON логи** | Структурированная статистика | `pinger -o stats.json` |

## 📊 **Пример вывода**

64 bytes from 192.168.1.1: icmp_seq=1 ttl=64 time=0.234 ms 🟢
64 bytes from 192.168.1.1: icmp_seq=2 ttl=64 time=1.456 ms 🟡

--- ping statistics ---
100 transmitted, 100 received, 0.0% packet loss
round-trip min/avg/max = 0.234ms/1.123ms/12.456ms
jitter Jitter: 0.789ms
bandwidth Bandwidth: 45.2 Mbps
frame Frame size: 98 bytes (ICMP data: 56)

**Traceroute:**
TRACE traceroute to 8.8.8.8 (8.8.8.8), 30 hops max
1 192.168.1.1 0.8ms
2 10.0.0.1 2.3ms
...
30 8.8.8.8 25.4ms DEST!

## 🎛 **Флаги**

| Флаг | Тип | По умолчанию | Описание | Пример |
|------|-----|--------------|----------|---------|
| `-c` | `int` | `0` | Кол-во пакетов (`0`=∞) | `-c 10` |
| `-i` | `duration` | `1s` | Интервал (min 1ms) | `-i 50ms` |
| `-o` | `string` | `""` | JSON файл статистики | `-o stats.json` |
| `-s` | `int` | `56` | ICMP данные (0-1472) | `-s 1472` |
| `--mtu-test` | `bool` | `false` | Авто MTU тест | `--mtu-test` |
| `-V` | `bool` | `false` | Версия | `-V` |
| `-v` | `bool` | `false` | Verbose stats | `-v` |
| `-live` | `bool` | `false` | Live статистика | `-live` |
| `-t` | `int` | `64` | IP TTL (1-255) | `-t 32` |
| `--trace` | `bool` | `false` | Traceroute режим | `--trace` |
| `-h` | - | - | Справка | `-h` |

## 🚀 **Быстрый старт**

Установка
go install github.com/yourusername/pinger@latest

Базовый ping
pinger -c 5 8.8.8.8

Нагрузка + все фичи
pinger -c 1000 -i 10ms -s 1400 -v -live --mtu-test -o loadtest.json 192.168.1.1

Traceroute
pinger --trace -t 64 google.com

Jumbo ping
pinger -s 9000 --mtu-test 192.168.1.1

## 💾 **JSON статистика** (`-o stats.json`)

{
"packets_sent": 100,
"packets_received": 99,
"packet_loss_percent": 1.0,
"min_rtt": 234125,
"avg_rtt": 1234567,
"max_rtt": 2456789,
"measurements": [{"seq":1,"rtt":234125},...]
}

## 🛠 **Сборка и запуск**

git clone <repo>
cd pinger
go mod tidy
go run .

или
go build -o pinger
./pinger -h

**Windows**: без прав админа  
**Linux**: `sudo` или `CAP_NET_RAW`

## 🎨 **Цвета**

🟢 **<1ms** | 🟡 **1-10ms** | 🔴 **>10ms**

## 📄 **Лицензия**
MIT

Функция	    Описание	                                                Пример
ICMP Ping	Классический ping с кастомным размером пакетов	            pinger -c 100 8.8.8.8
Traceroute	Построение маршрута с инкрементальным TTL	                pinger --trace -t 30 8.8.8.8
Jumbo/MTU	Тестирование больших пакетов (до 9K+)	                    pinger -s 1472 --mtu-test
Live        статистика	Онлайн мониторинг loss/RTT в реальном времени	pinger -live -v
JSON логи	Полная статистика в структурированный JSON	                pinger -o stats.json

Перечень флагов:
Флаг	    Тип	             По умолчанию	Описание	                                             Пример
-c	        int 	            0	        Количество пакетов для отправки.                         0 = бесконечно (до Ctrl+C)	-c 10 (10 пингов)
-i	        time.Duration	    1s	        Интервал между пакетами (минимум 1ms).                   Поддерживает: ms, s, m	-i 50ms, -i 1s
-o	        string	            ""	        Путь к JSON файлу для сохранения статистики в конце	     -o stats.json
-s	        int	                56	        Размер ICMP данных в байтах (0-1472). Полный пакет:      -s size+28	-s 1472 (jumbo)
-mtu-test	bool	            false	    Автоопределение MTU (тестит 1500, 9000, 12000). Меняет   --mtu-test
-V	        bool	            false	    Показать версию и выйти	                                 -V
-v	        bool	            false	    Расширенная статистика (jitter, bandwidth, frame size)	 -v
-live	    bool	            false	    Live статистика в реальном времени (каждые 10s)	         -live
-t	        int	64	            IP TTL (1-255). Для ping и traceroute max hops	                     -t 32
--trace	    bool	            false	    Traceroute режим: TTL от 1 до -t, показывает маршрут	--trace -t 30
-h                                          Help
