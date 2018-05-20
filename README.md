# Basic Usage

## Simple static peer list
```
bazel run //:bazel-cache-s3 -- -bucket <s3-bucket> -self http://localhost:8080 -peers http://localhost:8080,http://localhost:8081
```

## Simple peer discovery
```
bazel run //:baze-cache-s3 -- -bucket <s3-bucket> -self http://127.0.0.1:8080 -peer-endpoints localhost,locahost.example.com
```

## Simple peer discovery: Kubernetes Headless Service
```
bazel run //:baze-cache-s3 -- -bucket <s3-bucket> -self "http://$(ifconfig en0 |grep "inet " |awk '{ print $2 }'):8080" -peer-endpoints bazel-cache.default.svc.cluster.local.
```
