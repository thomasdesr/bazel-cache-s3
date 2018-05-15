load("@bazel_gazelle//:def.bzl", "gazelle")
load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

gazelle(
    name = "gazelle",
    prefix = "github.com/thomaso-mirodin/bazel-cache-s3",
)

go_library(
    name = "go_default_library",
    srcs = [
        "main.go",
        "peers.go",
        "s3.go",
        "server.go",
    ],
    importpath = "github.com/bazel-cache-s3",
    visibility = ["//visibility:private"],
    deps = [
        "@com_github_aws_aws_sdk_go//aws:go_default_library",
        "@com_github_aws_aws_sdk_go//aws/awserr:go_default_library",
        "@com_github_aws_aws_sdk_go//aws/session:go_default_library",
        "@com_github_aws_aws_sdk_go//service/s3:go_default_library",
        "@com_github_aws_aws_sdk_go//service/s3/s3manager:go_default_library",
        "@com_github_go_chi_chi//:go_default_library",
        "@com_github_go_chi_chi//middleware:go_default_library",
        "@com_github_golang_groupcache//:go_default_library",
        "@com_github_pkg_errors//:go_default_library",
        "@in_gopkg_tylerb_graceful_v1//:go_default_library",
    ],
)

go_binary(
    name = "bazel-cache-s3-linux",
    embed = [":go_default_library"],
    goarch = "amd64",
    goos = "linux",
    visibility = ["//visibility:public"],
)

go_binary(
    name = "bazel-cache-s3-darwin",
    embed = [":go_default_library"],
    goarch = "amd64",
    goos = "darwin",
    visibility = ["//visibility:public"],
)
