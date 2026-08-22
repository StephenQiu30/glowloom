SWAG_VERSION ?= v1.16.6

.PHONY: swagger

swagger:
	go run github.com/swaggo/swag/cmd/swag@$(SWAG_VERSION) init -g cmd/main.go -d . -o docs --outputTypes json --parseInternal
