Pinger — это инструмент для сетевой диагностики, написанный на Go. Объединяет функциональность ping, traceroute и MTU discovery в одном бинарнике с цветным выводом, JSON-логами и live-статистикой.
Написана для собственных нужд

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
