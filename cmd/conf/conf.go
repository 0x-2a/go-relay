package conf

const (
	ReadBufferSize  = 0
	WriteBufferSize = 0
	MessageChanSize = 2048

	SenderThrottleMillis = 20
	//SenderThrottleMillis = 1000

	PayloadMinBytes = 2 // must be less than max
	PayloadMaxBytes = 4096

	IgnoreInitialMessageCount = 100

	TimestampBytes = 8

	RandSeed = 42

	UseGosched   = true
	LockOSThread = false
)
