# zEngine
## simple tcp server engine

It was too simple, a few hundred line of code, so there was no introduction

but,It is powerful, stable and efficient

You can refer to the examples in the ./example

or refer to https://github.com/pzqf/zChatRoom

anything, issues.


# create rsa key file
## private key
```~#openssl genrsa -out rsa_private.key 2048```
## public key
```~#openssl rsa -in rsa_private.key -pubout -out rsa_public.key```