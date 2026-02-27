module github.com/fslongjin/liteboxd/sdk/go

go 1.24.0

require (
	github.com/fslongjin/liteboxd/backend v0.0.0-20260226055749-26fdd9e22436
	github.com/gorilla/websocket v1.5.3
)

replace github.com/fslongjin/liteboxd/backend => ../../backend
