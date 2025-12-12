#scp -P 22  nakama   root@39.101.186.196:/root/nakama/bin
#scp -P 22  nakama   root@192.168.102.223:/root/nakama/bin
set GOOS=linux
set GOARCH=amd64

go build -o nakama
scp -P 22  nakama   root@39.101.186.196:/root/dy-xjsm-iap/bin

#scp -P 31222  nakama   root@118.145.206.63:/root/zw_zjstf/bin

rem scp -P 31222  nakama   root@47.122.45.71:/root/nakama/bin
rem  scp -P 31222  nakama   root@8.138.94.100:/root/nakama/bin
rem  scp -P 22  nakama   root@8.138.113.90:/root/nakama/bin


