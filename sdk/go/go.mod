module github.com/fslongjin/liteboxd/sdk/go

go 1.23.4

require (
	github.com/fslongjin/liteboxd/backend v0.0.0-20260225161125-4de0622cb5d4
	github.com/gorilla/websocket v1.5.3
)

replace github.com/fslongjin/liteboxd/backend => ../../backend
