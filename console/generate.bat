@echo off
chcp 65001 >nul
REM Windows批处理脚本，用于生成console相关文件
REM 相当于执行 go generate 命令

echo 正在生成protobuf文件...
protoc -I. -I../vendor -I../game -I../vendor/github.com/heroiclabs/nakama-common -I../build/grpc-gateway-v2.3.0/third_party/googleapis -I../vendor/github.com/grpc-ecosystem/grpc-gateway/v2 --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative --grpc-gateway_out=. --grpc-gateway_opt=paths=source_relative --grpc-gateway_opt=logtostderr=true --grpc-gateway_opt=generate_unbound_methods=true --openapiv2_out=. --openapiv2_opt=json_names_for_fields=false,logtostderr=true console.proto

if %ERRORLEVEL% neq 0 (
    echo protobuf生成失败！
    exit /b %ERRORLEVEL%
)

echo 正在生成Angular服务文件...
go run ./openapi-gen-angular/main.go -i console.swagger.json -o ui/src/app/console.service.ts -rm_prefix=console,nakamaconsole,nakama,Console_

if %ERRORLEVEL% neq 0 (
    echo Angular服务生成失败！
    exit /b %ERRORLEVEL%
)

echo 生成完成！
