@echo off
REM Windows batch script to generate console proto files
REM Equivalent to: go generate

echo Generating protobuf files...
protoc -I. -I../vendor -I../game -I../vendor/github.com/heroiclabs/nakama-common -I../build/grpc-gateway-v2.3.0/third_party/googleapis -I../vendor/github.com/grpc-ecosystem/grpc-gateway/v2 --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative --grpc-gateway_out=. --grpc-gateway_opt=paths=source_relative --grpc-gateway_opt=logtostderr=true --grpc-gateway_opt=generate_unbound_methods=true --openapiv2_out=. --openapiv2_opt=json_names_for_fields=false,logtostderr=true console.proto

if %ERRORLEVEL% neq 0 (
    echo ERROR: protobuf generation failed!
    exit /b %ERRORLEVEL%
)

echo Generating Angular service...
go run ./openapi-gen-angular/main.go -i console.swagger.json -o ui/src/app/console.service.ts -rm_prefix=console,nakamaconsole,nakama,Console_

if %ERRORLEVEL% neq 0 (
    echo ERROR: Angular service generation failed!
    exit /b %ERRORLEVEL%
)

echo SUCCESS: All files generated successfully!
