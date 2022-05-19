go build -buildmode=plugin ../mrapps/wc.go
rm mr-*
go run mrcoordinator.go pg-*.txt
go run mrworker.go wc.so

cat mr-out-* | sort | more
