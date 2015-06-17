string_keeper
=============

 Sometimes, we sorage some text file at server, like nginx, e.g.: shell script, python and so on,
 but we want it can be replace some `text` will we tell the server, 
 it was usefull for storage shell script and run mutil server, we could pass the `serverName`, `branch`, `commitID` and just what you want to replace with. 


We do this by golang tmplate, the client side post the file and key-values to server, the server will build by golang template and replace the values to the raw text stirng


### Usage

Do some preper

1. create text file

create file: `namespace/bucket/dir1/dir2/abc.sh` in public dir

content of `namespace/bucket/dir1/dir2/abc.sh`

> you can use any name with `namespace` and `bucket` in real world

```
#!/bin/bash
echo hello
echo {{.hello}}
```

2. configure the `conf/string_keeper.conf`

```json
{
    "http": {
        "address": ":8080",
        "cors": {
            "allow_origins": ["http://*.gogap.cn"],
            "allow_methods": ["POST"],
            "allow_headers": ["Origin"],
            "expose_headers": ["Content-Length"],
            "allow_cerdentials": false
        }
    },
    "acl": {
        "ip_acl_enabled": true,
        "ip_white_list": ["127.0.0.1"],
        "auth_enabled": true,
        "basic_auth": {
            "namespace/bucket": "token"
        }
    }
}
```

> we have ip white list and bucket auth, it was safe for only deply script

3. start `string_keeper`

```bash
$ go build
$ ./string_keeper
```


The post data like following:

```json
{
    "namespace": "namespace",
    "bucket": "bucket",
    "file": "dir1/dir2/abc.sh",
    "envs": {
        "hello": "world"
    },
    "raw_data": false
}
```

`envs` is an `key-value` style, the key is in the template file like `{{.hello}}`, and it will use go template and replace the `world` with it.

if `raw_data` is true, we will get the raw template, not replaced with envs values.

Take a look with `curl`

```bash
$ curl -X POST --basic -u "namespace/bucket:token" -d '{
"namespace":"namespace",
"bucket":"bucket",
"file":"dir1/dir2/abc.sh",
"envs": {"hello":"world"},
"raw_data":false
}' http://127.0.0.1:8080/
```

we got this:

```bash
#!/bin/bash
echo hello
echo world
```

change the "raw_data" to false, we got this:

```
#!/bin/bash
echo hello
echo {{.hello}}
```
