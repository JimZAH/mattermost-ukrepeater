build:
	GOOS=linux GOARCH=amd64 go build -o plugin.exe plugin.go


compile: build
	tar -czvf plugin.tar.gz plugin.exe plugin.json

all: compile
