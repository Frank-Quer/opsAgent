.PHONY: build check test

build:
	go build -o bin/opsagent .

check:
	python3 scripts/check.py

test:
	python3 scripts/check.py test
