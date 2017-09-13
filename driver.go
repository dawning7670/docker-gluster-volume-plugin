package main

import (
	"sync"
	"github.com/coreos/etcd/client"
	"github.com/docker/go-plugins-helpers/volume"
	"log"
	"os"
	"encoding/json"
	"io/ioutil"
	"time"
	"path/filepath"
	"fmt"
	"os/exec"
	"context"
)

type EtcdEvent struct {
	Action            string `json:"action"`
	GlusterVolumeName string `json:"gluster_volume_name"`
	MountName         string `json:"mount_name"`
}
type GlusterVolume struct {
	Name        string `json:"name"`
	MountName   string `json:"mount_name"`
	MountPoint  string `json:"mount_path"`
	Connections int    `json:"connections"`
}

type GlusterDriver struct {
	sync.Mutex
	Server  string
	BaseDir string
	Volumes map[string]*GlusterVolume
	EtcdAPI client.KeysAPI
}

func Init(server string, baseDir string, etcdUrls []string) GlusterDriver {
	d := GlusterDriver{
		Server:  server,
		BaseDir: baseDir,
		Volumes: make(map[string]*GlusterVolume),
	}
	etcdApi, err := CreateEtcdApi(etcdUrls)
	if err != nil {
		log.Fatal(err)
	}
	d.EtcdAPI = etcdApi
	if exists, _ := PathExists(PersistenceFilePath); exists {
		log.Printf("found persist file %s, retrieve config", PersistenceFilePath)
		volumes, _ := ReadPersistFile()
		d.Volumes = volumes
	} else {
		log.Printf("not found persist file %s, it will be created empty", PersistenceFilePath)
	}
	go d.EventWatcher()
	return d
}

func (d GlusterDriver) Create(r *volume.CreateRequest) error {
	d.Lock()
	defer d.Unlock()
	log.Printf("Entering Create")
	if _, ok := r.Options[OptGlusterVolumeName]; !ok {
		return fmt.Errorf("vname option required")
	}
	name, _ := r.Options[OptGlusterVolumeName]
	log.Printf("create volume %s, %s, %s", r.Name, OptGlusterVolumeName, name)
	d.Volumes[r.Name] = &GlusterVolume{
		Name:        name,
		MountName:   r.Name,
		Connections: 0,
		MountPoint:  d.MountPoint(r.Name),
	}

	if exists, _ := PathExists(d.MountPoint(r.Name)); !exists {
		err := os.MkdirAll(d.MountPoint(r.Name), 0700)
		if err != nil {
			return err
		}
	}
	rspn, err := d.NotifyServers(EtcdEvent{Action: EventActionCreate, GlusterVolumeName: name, MountName: r.Name})
	log.Printf("notify create event. rspn: %s, err: %s", rspn, err)
	WritePersistFile(d.Volumes)
	return nil
}
func (d GlusterDriver) List() (*volume.ListResponse, error) {
	d.Lock()
	defer d.Unlock()
	log.Printf("Entering List")
	var volumes []*volume.Volume
	for _, v := range d.Volumes {
		volumes = append(volumes, &volume.Volume{Name: v.MountName, Mountpoint: v.MountPoint})
	}
	return &volume.ListResponse{Volumes: volumes}, nil
}
func (d GlusterDriver) Get(r *volume.GetRequest) (*volume.GetResponse, error) {
	d.Lock()
	defer d.Unlock()
	log.Printf("Entering Get %s", r.Name)
	if v, ok := d.Volumes[r.Name]; ok {
		return &volume.GetResponse{Volume: &volume.Volume{Name: r.Name, Mountpoint: v.MountPoint}}, nil
	}
	return &volume.GetResponse{}, fmt.Errorf("not found volume %s", r.Name)
}
func (d GlusterDriver) Remove(r *volume.RemoveRequest) error {
	d.Lock()
	defer d.Unlock()
	log.Printf("Entering Remove %s", r.Name)
	if _, ok := d.Volumes[r.Name]; !ok {
		return fmt.Errorf("not found volume %s", r.Name)
	}
	delete(d.Volumes, r.Name)
	rspn, err := d.NotifyServers(EtcdEvent{Action: EventActionRemove, MountName: r.Name})
	log.Printf("notify create event. rspn: %s, err: %s", rspn, err)
	WritePersistFile(d.Volumes)
	return nil
}

func (d GlusterDriver) Path(r *volume.PathRequest) (*volume.PathResponse, error) {
	d.Lock()
	defer d.Unlock()
	log.Printf("Entering Path %s", r.Name)
	return &volume.PathResponse{Mountpoint: d.MountPoint(r.Name)}, nil
}

func (d GlusterDriver) Mount(r *volume.MountRequest) (*volume.MountResponse, error) {
	d.Lock()
	defer d.Unlock()
	log.Printf("Entering Mount %s, %s", r.Name, r.ID)
	v, ok := d.Volumes[r.Name]
	if !ok {
		return &volume.MountResponse{}, fmt.Errorf("not found volume %s", r.Name)
	}
	if v.Connections == 0 {
		uri := fmt.Sprintf("%s:%s", d.Server, v.Name)
		cmd := fmt.Sprintf("/usr/bin/mount -t glusterfs %s %s", uri, v.MountPoint)
		err := ExecuteCommand(cmd)
		log.Printf("execute %s", cmd)
		if err != nil {
			return &volume.MountResponse{}, err
		}
	}

	v.Connections ++
	WritePersistFile(d.Volumes)
	return &volume.MountResponse{Mountpoint: v.MountPoint}, nil
}

func (d GlusterDriver) Unmount(r *volume.UnmountRequest) error {
	d.Lock()
	defer d.Unlock()
	log.Printf("Entering Unmount %s, %s", r.Name, r.ID)
	v, ok := d.Volumes[r.Name]
	if !ok {
		return fmt.Errorf("not found volume %s", r.Name)
	}
	if v.Connections > 1 {
		v.Connections--
	} else {
		v.Connections = 0
		cmd := fmt.Sprintf("/usr/bin/umount %s", v.MountPoint)
		err := ExecuteCommand(cmd)
		log.Printf("execute %s", cmd)
		if err != nil {
			return err
		}
	}
	WritePersistFile(d.Volumes)
	return nil
}

func (d GlusterDriver) Capabilities() *volume.CapabilitiesResponse {
	return &volume.CapabilitiesResponse{
		Capabilities: volume.Capability{
			Scope: "local",
		},
	}
}

func (d GlusterDriver) MountPoint(name string) string {
	return filepath.Join(d.BaseDir, name)
}

func ReadPersistFile() (map[string]*GlusterVolume, error) {
	var volumes map[string]*GlusterVolume
	jsonStr, err := ioutil.ReadFile(PersistenceFilePath)
	json.Unmarshal(jsonStr, &volumes)
	return volumes, err
}

func WritePersistFile(volumes map[string]*GlusterVolume) error {
	bytes, err := json.Marshal(volumes)
	err = ioutil.WriteFile(PersistenceFilePath, bytes, os.ModePerm)
	return err
}

func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func CreateEtcdApi(etcdUrls []string) (client.KeysAPI, error) {
	cfg := client.Config{
		Endpoints: etcdUrls,
		Transport: client.DefaultTransport,
		// set timeout per request to fail fast when the target endpoint is unavailable
		HeaderTimeoutPerRequest: time.Second,
	}
	c, err := client.New(cfg)
	kapi := client.NewKeysAPI(c)
	return kapi, err
}

func ExecuteCommand(cmd string) error {
	return exec.Command("sh", "-c", cmd).Run()
}

func (d GlusterDriver) EventWatcher() {
	watcher := d.EtcdAPI.Watcher(EtcdEventUrl, &client.WatcherOptions{Recursive: true})
	for {
		res, err := watcher.Next(context.Background())
		if err != nil {
			log.Println("Error watch workers:", err)
			break
		}
		if res.Action == "set" {
			d.Lock()
			var event EtcdEvent
			json.Unmarshal([]byte(res.Node.Value), &event)
			if event.Action == EventActionCreate {
				log.Printf("received create action")
				cmd := fmt.Sprintf("docker volume create --driver %s --opt vname=%s --name %s",
					PluginID, event.GlusterVolumeName, event.MountName)
				log.Printf("execute %s", cmd)
				ExecuteCommand(cmd)
				d.Volumes[event.MountName] = &GlusterVolume{
					Name:        event.GlusterVolumeName,
					MountName:   event.MountName,
					Connections: 0,
					MountPoint:  d.MountPoint(event.MountName),
				}
				WritePersistFile(d.Volumes)
			} else if event.Action == EventActionRemove {
				log.Printf("received delete action")
				cmd := fmt.Sprintf("docker volume rm %s", event.MountName)
				log.Printf("execute %s", cmd)
				ExecuteCommand(cmd)
				delete(d.Volumes, event.MountName)
				WritePersistFile(d.Volumes)
			}
			d.Unlock()
		}
	}
}

func (d GlusterDriver) NotifyServers(event EtcdEvent) (*client.Response, error) {
	eventBytes, _ := json.Marshal(event)
	return d.EtcdAPI.Set(context.Background(), EtcdEventUrl, string(eventBytes), nil)
}
