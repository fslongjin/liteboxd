module github.com/fslongjin/liteboxd/sdk/go

go 1.24.0

require (
	github.com/fslongjin/liteboxd/backend v0.0.0-20260317080906-c0a0ca7fd05a
	github.com/gorilla/websocket v1.5.3
)

replace github.com/fslongjin/liteboxd/backend => ../../backend
