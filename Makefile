.PHONY: demounits

all: demounits

demounits:
	go build -v -o _demo/unit_01/unit_01 coprtest/cmd/test_dummy/main.go
	go build -v -o _demo/unit_02/unit_02 coprtest/cmd/test_dummy/main.go