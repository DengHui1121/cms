go build -o ./build/windows/cmsProgram.exe ./handler
go build -o ./build/windows/cms.exe ./main.go
go build -o ./build/windows/analysis/cmsModbus.exe ./modbusexe/modbus.go
go build -o ./build/windows/analysis/cmsDatawatch.exe ./insertexe/insert.go ./insertexe/insertex_windows.go
