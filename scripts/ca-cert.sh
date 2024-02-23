# Generate server.key and server.crt signed by our local CA. 
openssl genrsa -out $PWD/keys/server.key 2048

openssl req -sha256 -new -key $PWD/keys/server.key -out $PWD/keys/server.csr \
  -subj "/CN=localhost" \

openssl x509 -req -in $PWD/keys/server.csr -CA $PWD/keys/my-own-ca.crt -CAkey $PWD/keys/my-own-ca.key \
-CAcreateserial -out $PWD/keys/server.crt -days 825 -sha256 -extfile $PWD/keys/server.ext

# Confirm the certificate is valid. 
openssl verify -CAfile $PWD/keys/my-own-ca.crt $PWD/keys/server.crt
