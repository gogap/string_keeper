string_keeper
=============

 Sometimes, we store some text file at server, like nginx, e.g.: shell script, python and so on, then we will do something like:
 `curl www.gogap.cn/xxx.sh | sh` for install or run something, 
but we want it can be replaced by some values that we told the server, 
it was usefull for storage shell script and run mutil server, we could pass the `serverName`, `branch`, `commitID` and just what you want to replace with. 


We could do this by golang tmplate, the client side post the file name and key-values to server, the server will build by golang template then replace the values to the raw text stirng


### Usage

Do some preper

**create text file**

create file: `namespace/bucket/dir1/dir2/abc.sh` in public dir

content of `namespace/bucket/dir1/dir2/abc.sh`

> you can use any name with `namespace` and `bucket` in real world

```
#!/bin/bash
echo hello
echo {{.hello}}
```

**configure the `conf/string_keeper.conf`**

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

**start `string_keeper`**

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


### Our practice

We are using gitlab-ci for continuous integration, and we have about 20 components project, and these project had same test and deploy shell script, storage the script at string_keeper, and put following bash script in gitlab-ci deploy jobs.

**gitlab-ci deploy job**

```bash
curl -X POST --basic -u "ci-scripts-development/components:password" -d '{
    "namespace": "ci-scripts-development",
    "bucket": "components",
    "file": "common/build_and_run.sh",
    "raw_data":false,
    "envs":{
           "gopath":"/gopath",
           "launchboard_path":"/launchboard/components",
           "package":"git.xxx.com/components/sms",
           "component_name":"sms",
           "compose_name":"sms",
           "brunch":"develop" 
    }
}' https://string-keeper.xxx.com | sh
```

**deploy script**

File:`ci-scripts-development/components/common/build_and_run.sh`

```bash
#/bin/bash
cd  {{.gopath}}/src/{{.package}}
git checkout {{.brunch}}

cd {{.launchboard_path}}/{{.component_name}}
make
cd ..
docker-compose build {{.compose_name}}
docker-compose stop {{.compose_name}}
docker-compose rm -f {{.compose_name}}
docker-compose up -d {{.compose_name}}
pid=$(docker inspect --format='{{"{{.State.Pid}}"}}' components_{{.compose_name}}_1)
if [ $pid = "0"  ]; then
    exit  3
fi
```
