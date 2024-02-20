# create a ca certificate

openssl genrsa -out $PWD/config-secure/keys/my-own-ca.key 2048
openssl req -new -x509 -days 3650 -key $PWD/config-secure/keys/my-own-ca.key \
  -subj "/CN=qdb_root" \
  -addext "subjectAltName = DNS:qdb_root" \
  -sha256 -extensions v3_ca -out $PWD/config-secure/keys/my-own-ca.crt
