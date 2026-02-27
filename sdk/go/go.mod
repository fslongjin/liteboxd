module github.com/fslongjin/liteboxd/sdk/go

go 1.24.0

require (
	github.com/fslongjin/liteboxd/backend v0.0.0-20260227061216-49dc84ae0479
	github.com/gorilla/websocket v1.5.3
)

replace github.com/fslongjin/liteboxd/backend => ../../backend
