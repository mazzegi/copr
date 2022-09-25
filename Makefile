.PHONY: demounits

all: demounits

demounits:
	go build -v -o _demo/unit_01/unit_01 coprtest/cmd/test_dummy/main.go
	go build -v -o _demo/unit_02/unit_02 coprtest/cmd/test_dummy/main.go

secrettools:
	go build -v -o $(HOME)/bin/seced  cmd/seced/main.go
	go build -v -o $(HOME)/bin/seccat cmd/seccat/main.go
