.PHONY: build test vet e2e clean

build:
	go build -o zfw .

test:
	go test ./...

vet:
	go vet ./...

e2e:
	@test -n "$$ZFW_E2E_HOST" || (echo "set ZFW_E2E_HOST=hostname"; exit 1)
	go run . info $$ZFW_E2E_HOST

clean:
	rm -f zfw
