package caster

import (
    "fmt"
    "net/http"
    "log"
    "context"
    "github.com/satori/go.uuid"
)

func (mount *Mountpoint) BroadcastStream() { // Should this be a method of Mountpoint?
    fmt.Fprintf(mount.Source.Writer, "\r\n")
    mount.Source.Writer.(http.Flusher).Flush()

    buf := make([]byte, 1024)
    _, err := mount.Source.Request.Body.Read(buf)
    for ; err == nil; _, err = mount.Source.Request.Body.Read(buf) {
        mount.Broadcast(buf)
        buf = make([]byte, 1024)
    }

    mount.Lock()
    for _, client := range mount.Clients {
        client.Cancel()
    }
    mount.Unlock()
}

func Serve() {
    mounts := MountpointCollection{mounts: make(map[string]*Mountpoint)}

    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        requestId := uuid.Must(uuid.NewV4()).String()
        w.Header().Set("X-Request-Id", requestId)
        w.Header().Set("Ntrip-Version", "Ntrip/2.0")
        w.Header().Set("Server", "NTRIP GoCaster")

        ctx, cancel := context.WithCancel(r.Context())
        // Not sure how large to make the buffered channel
        client := Client{requestId, make(chan []byte, 5), r, w, ctx, cancel}

        switch r.Method {
            case http.MethodPost:
                mount, err := mounts.NewMountpoint(client)
                if err != nil {
                    w.WriteHeader(http.StatusConflict)
                    return
                }

                log.Println("Mountpoint connected:", mount.Source.Request.URL.Path)
                mount.BroadcastStream()
                log.Println("Mountpoint disconnected:", mount.Source.Request.URL.Path, err)
                mounts.DeleteMountpoint(mount.Source.Request.URL.Path)

            case http.MethodGet:
                if mount, exists := mounts.GetMountpoint(r.URL.Path); exists {
                    mount.AddClient(&client) // Can this fail?
                    log.Println("Accepted Client on mountpoint", client.Request.URL.Path)
                    client.Listen()
                    log.Println("Client disconnected", client.Id)
                    mount.DeleteClient(client.Id)
                } else {
                    w.WriteHeader(http.StatusNotFound)
                }

            default:
                w.WriteHeader(http.StatusNotImplemented)
        }
    })

    log.Fatal(http.ListenAndServe(":2101", nil))
}
