# Generate server.key and server.crt signed by our local CA. 
openssl genrsa -out $PWD/config-secure/keys/server.key 2048

openssl req -sha256 -new -key $PWD/config-secure/keys/server.key -out $PWD/config-secure/keys/server.csr \
  -subj "/CN=localhost" \

openssl x509 -req -in $PWD/config-secure/keys/server.csr -CA $PWD/config-secure/keys/my-own-ca.crt -CAkey $PWD/config-secure/keys/my-own-ca.key \
-CAcreateserial -out $PWD/config-secure/keys/server.crt -days 825 -sha256 -extfile $PWD/config-secure/keys/server.ext

# Confirm the certificate is valid. 
openssl verify -CAfile $PWD/config-secure/keys/my-own-ca.crt $PWD/config-secure/keys/server.crt
