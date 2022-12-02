
# Encryption process
1. server send "hello" to client
2. after client received "hello", client create aesKey, aesKey = md5("aes$(time.Now().UnixNano())") 
3. client Encrypt aes key by rsa public key
4. send Encrypted aes key to server
5. after server received Encrypted aes key, Decrypted it by rsa private key.
6. from then on, the client and server communicate using aes encryption

# create rsa key file
## private key
```
~#openssl genrsa -out rsa_private.key 2048
```

## public key
```
~#openssl rsa -in rsa_private.key -pubout -out rsa_public.key
```
