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
	persistenceFilePath = "/etc/docker-gluster-volume-plugin-persistence.json"
	pluginID            = "glusterfs"
)

var (
	defaultDir  = filepath.Join(volume.DefaultDockerRootDirectory, pluginID)
	server      = flag.String("server", "", "one server name from servers")
	baseDir     = flag.String("basedir", defaultDir, "GlusterFS volumes root directory")
	etcdServers = flag.String("etcd", "", "etcd server address, more than one servers separator with comma")
)

func main() {
	var Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()
	if len(*server) == 0 {
		Usage()
		os.Exit(1)
	}

	etcdUrls := strings.Split(*etcdServers, ",")

	d := Init(*server, *baseDir, etcdUrls)
	h := volume.NewHandler(d)
	u, _ := user.Lookup("root")
	gid, _ := strconv.Atoi(u.Gid)
	h.ServeUnix(pluginID, gid)
}
