#!/bin/bash
# Arranca, para o reinicia Tabernacle en local, con los datos en ./data.
#
# No forma parte del despliegue: para un servidor de verdad usa compose.yaml.
# Esto es para tenerlo a mano mientras se desarrolla.
set -e
cd "$(dirname "$0")"

BIN="${TABERNACLE_BIN:-$HOME/.local/bin/tabernacle}"
DATA="$PWD/data"
LOG="$DATA/tabernacle.log"
export TABERNACLE_DATA_DIR="$DATA"
export TABERNACLE_ADDR="${TABERNACLE_ADDR:-:8080}"

# Por nombre exacto de proceso: buscar por línea de comandos completa acaba
# encontrando el propio grep o el editor que tenga el nombre abierto.
pid() { pgrep -x tabernacle 2>/dev/null | head -1; }

case "${1:-start}" in
start)
	if [ -n "$(pid)" ]; then echo "ya está corriendo (pid $(pid))"; exit 0; fi
	mkdir -p "$DATA/songs"
	setsid nohup "$BIN" > "$LOG" 2>&1 < /dev/null &
	disown 2>/dev/null || true
	for _ in $(seq 1 60); do
		curl -sf "http://127.0.0.1${TABERNACLE_ADDR}/healthz" > /dev/null 2>&1 && break
		sleep 0.2
	done
	echo "arriba: http://localhost${TABERNACLE_ADDR}"
	;;
stop)
	p="$(pid)"
	if [ -z "$p" ]; then echo "no estaba corriendo"; exit 0; fi
	kill -TERM "$p"
	echo "parado (pid $p)"
	;;
restart)
	"$0" stop || true
	sleep 0.5
	"$0" start
	;;
status)
	p="$(pid)"
	if [ -z "$p" ]; then echo "parado"; exit 1; fi
	echo "corriendo (pid $p)"
	curl -s "http://127.0.0.1${TABERNACLE_ADDR}/healthz"; echo
	;;
log)
	tail -f "$LOG"
	;;
build)
	# Recompila con el Go que tengas en el PATH.
	go build -trimpath -ldflags "-s -w -X main.version=dev" -o "$BIN" ./cmd/tabernacle
	echo "compilado en $BIN"
	;;
*)
	echo "uso: $0 {start|stop|restart|status|log|build}" >&2
	exit 2
	;;
esac
