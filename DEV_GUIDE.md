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

## Building the release artifact

⚠️ **Important:** The plugin has to be built from the `main` branch, if intended to be released into Grafana as version update. This is because the automated review process task compares the source tree inside the artifact with the current `main` branch of the repo, and fails if they don't match.

The final plugin artifact has to be signed either by a key using the [@grafana/sign-plugin](https://www.npmjs.com/package/@grafana/sign-plugin) tool. The script needs `GRAFANA_ACCESS_POLICY_TOKEN` ENV variable to be set before hand - it can be obtained in Grafana Cloud's personal account. 

By default, all the assets are built into `dist` directory, which does not match the Grafana's required one, which should match the plugin ID (in this case, `questdb-questdb-datasource`). Therefore, we need to proceed as following:

```sh
export GRAFANA_ACCESS_POLICY_TOKEN=your_token
nvm use 20
yarn build
mage -v buildAll    
cp -r dist/ questdb-questdb-datasource
npx @grafana/sign-plugin@latest --distDir questdb-questdb-datasource
zip -r questdb-questdb-datasource.zip questdb-questdb-datasource -r
md5 questdb-questdb-datasource.zip 
rm -rf questdb-questdb-datasource
```

`md5` checksum is needed only during the process of releasing the plugin version update in Grafana Cloud.

If intended to release into Grafana, the ZIP file has to be uploaded into a publicly available server (i.e. S3 bucket), since the link to it has to be provided during the update process.
