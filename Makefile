BIN_DIR := bin

RECEIVER_SRC := cmd/receiver/receiver.go
RELAY_SRC := cmd/relay/relay.go
SENDER_SRC := cmd/sender/sender.go

RECEIVER_BIN := $(BIN_DIR)/receiver
RELAY_BIN := $(BIN_DIR)/relay
SENDER_BIN := $(BIN_DIR)/sender

.PHONY: build
build: $(RECEIVER_BIN) $(RELAY_BIN) $(SENDER_BIN)

$(RECEIVER_BIN): $(RECEIVER_SRC)
	go build -o $(RECEIVER_BIN) $(RECEIVER_SRC)

$(RELAY_BIN): $(RELAY_SRC)
	go build -o $(RELAY_BIN) $(RELAY_SRC)

$(SENDER_BIN): $(SENDER_SRC)
	go build -o $(SENDER_BIN) $(SENDER_SRC)

.PHONY: start
start:
	make stop
	make build

	echo "" >> relay.log
	./$(SENDER_BIN) &
	./$(RELAY_BIN) &
	./$(RECEIVER_BIN) >> relay.log 2>&1 &
	@echo ""
	@echo "Logs output to relay.log."
	@echo ""
	@echo "Kill using \`make stop\`"
	@echo ""

.PHONY: stop
stop:
	pkill -f $(RECEIVER_BIN) || true
	pkill -f $(RELAY_BIN) || true
	pkill -f $(SENDER_BIN) || true
