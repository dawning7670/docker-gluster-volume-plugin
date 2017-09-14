package main

import (
	"path/filepath"
	"github.com/docker/go-plugins-helpers/volume"
	"flag"
	"fmt"
	"os"
	"strings"
	"os/user"
	"strconv"
)

const (
	PersistenceFilePath  = "/etc/docker-gluster-volume-plugin-persistence.json"
	PluginID             = "glusterfs"
	EtcdEventUrl         = "/docker/gluster/volume/plugin/event"
	OptGlusterVolumeName = "vname"
	EventCreate          = "create"
	EventRemove          = "remove"
)

var (
	defaultDir  = filepath.Join(volume.DefaultDockerRootDirectory, PluginID)
	servers      = flag.String("server", "", "glusterfs servers. eg: server1:server2:server3")
	baseDir     = flag.String("basedir", defaultDir, "GlusterFS volumes root directory")
	etcdServers = flag.String("etcd", "", "etcd server address, more than one servers separator with comma")
)

func main() {
	var Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()
	if len(*servers) == 0 {
		Usage()
		os.Exit(1)
	}

	etcdUrls := strings.Split(*etcdServers, ",")
	serverList := strings.Split(*servers, ":")

	d := Init(serverList, *baseDir, etcdUrls)
	h := volume.NewHandler(d)
	u, _ := user.Lookup("root")
	gid, _ := strconv.Atoi(u.Gid)
	h.ServeUnix(PluginID, gid)
}
