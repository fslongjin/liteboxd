module code-interpreter-example

go 1.23.4

require github.com/fslongjin/liteboxd/sdk/go v0.0.0

require (
	github.com/fslongjin/liteboxd/backend v0.0.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
)

replace github.com/fslongjin/liteboxd/sdk/go => ../../../sdk/go

replace github.com/fslongjin/liteboxd/backend => ../../../backend
