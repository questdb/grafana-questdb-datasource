# Guide to get QuestDB running

## Add a directory where the database files will mount to from your docker container

mkdir $HOME/workspace/questdb/db/

## run questdb - expose ports and add a volume to your folder above

docker run -d -p 8812:8812 -p 9000:9000 --name grafana-questdb-server --ulimit nofile=262144:262144 --volume=$HOME/workspace/questdb/db:/var/lib/questdb/db questdb/questdb

## Connect from the Plugin (minimum requirements)

server address: grafana-questdb-server
server port: 8812

## With custom config

docker run -d -p 8812:8812 -p 9000:9000 --name secure-questdb-server --ulimit nofile=262144:262144 -v $PWD/config/server.conf:/var/lib/questdb/conf/server.conf questdb/questdb

## With secure config - for testing TLS scenarios

### First set up the certificates

1. Create the CA cert

```
./scripts/ca.sh
```

2. Create the Server cert from the CA

```
./scripts/ca-cert.sh
```

3. The Common/SAN name is "qdb". Add an entry to your hosts file on the host.

```
127.0.0.1  qdb
```

### Now start the container using the config-secure settings

docker run -d -p 9000:9000 -p 8812:8812 --name secure-questdb-server --ulimit nofile=262144:262144 -v $PWD/keys:/var/lib/questdb/conf/keys -v $PWD/config-secure/server.conf:/var/lib/questdb/conf/server.conf questdb/questdb-enterprise

### Login to the container and add the ca cert to trusted certs

docker exec -it secure-questdb-server bash
cp /var/lib/questdb/conf/keys/my-own-ca.crt /usr/local/share/ca-certificates/root.ca.crt
update-ca-certificates
