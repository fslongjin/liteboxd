module github.com/fslongjin/liteboxd/sdk/go

go 1.23.4

require (
	github.com/fslongjin/liteboxd/backend v0.0.0-20260224055458-9ad3c0cf17f5
	github.com/gorilla/websocket v1.5.3
)

replace github.com/fslongjin/liteboxd/backend => ../../backend
