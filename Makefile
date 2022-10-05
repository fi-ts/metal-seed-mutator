all: metal-seed-mutator

.PHONY: metal-seed-mutator
metal-seed-mutator:
	go build mutator.go
	strip mutator
