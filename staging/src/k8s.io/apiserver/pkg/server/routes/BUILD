package(default_visibility = ["//visibility:public"])

load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "doc.go",
        "flags.go",
        "index.go",
        "metrics.go",
        "openapi.go",
        "openstack.go",
        "profiling.go",
        "version.go",
    ],
    importmap = "k8s.io/kubernetes/vendor/k8s.io/apiserver/pkg/server/routes",
    importpath = "k8s.io/apiserver/pkg/server/routes",
    deps = [
        "//staging/src/k8s.io/apimachinery/pkg/apis/meta/v1:go_default_library",
        "//staging/src/k8s.io/apimachinery/pkg/util/json:go_default_library",
        "//staging/src/k8s.io/apimachinery/pkg/util/sets:go_default_library",
        "//staging/src/k8s.io/apimachinery/pkg/version:go_default_library",
        "//staging/src/k8s.io/apiserver/pkg/endpoints/filters:go_default_library",
        "//staging/src/k8s.io/apiserver/pkg/endpoints/handlers/responsewriters:go_default_library",
        "//staging/src/k8s.io/apiserver/pkg/endpoints/metrics:go_default_library",
        "//staging/src/k8s.io/apiserver/pkg/server/mux:go_default_library",
        "//staging/src/k8s.io/apiserver/pkg/storage/etcd3/metrics:go_default_library",
        "//staging/src/k8s.io/apiserver/pkg/util/openstack:go_default_library",
        "//vendor/github.com/emicklei/go-restful:go_default_library",
        "//vendor/github.com/go-openapi/spec:go_default_library",
        "//vendor/github.com/prometheus/client_golang/prometheus:go_default_library",
        "//vendor/k8s.io/klog:go_default_library",
        "//vendor/k8s.io/kube-openapi/pkg/builder:go_default_library",
        "//vendor/k8s.io/kube-openapi/pkg/common:go_default_library",
        "//vendor/k8s.io/kube-openapi/pkg/handler:go_default_library",
    ],
)

filegroup(
    name = "package-srcs",
    srcs = glob(["**"]),
    tags = ["automanaged"],
    visibility = ["//visibility:private"],
)

filegroup(
    name = "all-srcs",
    srcs = [":package-srcs"],
    tags = ["automanaged"],
)

go_test(
    name = "go_default_test",
    srcs = ["index_test.go"],
    embed = [":go_default_library"],
    deps = [
        "//staging/src/k8s.io/apimachinery/pkg/apis/meta/v1:go_default_library",
        "//staging/src/k8s.io/apiserver/pkg/authentication/user:go_default_library",
        "//staging/src/k8s.io/apiserver/pkg/endpoints/request:go_default_library",
    ],
)
