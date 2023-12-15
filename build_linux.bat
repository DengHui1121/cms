set GOARCH=amd64
set GOOS=linux
set CGO_ENABLED=0
go build -o ./build/linux/cmsProgram ./handler
go build -o ./build/linux/cms ./main.go
go build -o ./build/linux/analysis/cmsModbus ./modbusexe/modbus.go
go build -o ./build/linux/analysis/cmsDatawatch ./insertexe/insert.go ./insertexe/insertex_linux.go