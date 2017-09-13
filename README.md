# docker-gluster-volume-plugin

Docker volume plugin for glusterfs

### Build

```sh
glide install
go build
```

### Usage

Start volume plugin, `server` is your gluster node name(only use one node name)

```sh
docker-gluster-volume-plugin -server server51
```

Create volume, the `vname`is your gluster volume name

```
docker volume create --driver glusterfs --opt vname="gv5" --name d-gv5
```

Run container

```sh
docker run --name test -v d-gv5:/data -td ubuntu:14.04.3
```

## Reference

[]: https://github.com/sapk/docker-volume-gluster
[]: https://github.com/calavera/docker-volume-glusterfs