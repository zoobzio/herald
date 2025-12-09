module github.com/zoobzio/herald/bolt

go 1.24.0

toolchain go1.24.5

require (
	github.com/zoobzio/herald v0.0.0-00010101000000-000000000000
	go.etcd.io/bbolt v1.3.11
)

require (
	github.com/stretchr/testify v1.11.1 // indirect
	github.com/zoobzio/capitan v0.0.9 // indirect
	github.com/zoobzio/clockz v0.0.2 // indirect
	github.com/zoobzio/pipz v0.0.19 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
)

replace github.com/zoobzio/herald => ../
