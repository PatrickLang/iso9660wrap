iso9660wrap
===========
This turns the [iso9660wrap](https://github.com/johto/iso9660wrap) utility into a package. It provides a simple means to create an ISO9660 file containing a single file. 



## Building with Bazel


```bash
bazel build //...
```


### Centos prerequisites

```bash
sudo curl https://copr.fedorainfracloud.org/coprs/vbatts/bazel/repo/epel-7/vbatts-bazel-epel-7.repo -o /etc/yum.repos.d/vbatts-bazel-epel-7.repo
yum install git bazel gcc libstdc++-static vim tmux
```

### Windows prerequisites

- Visual Studio 2017 community edition, "Desktop Apps" selected to get C++ tools
- Install bazel with `choco install bazel`
