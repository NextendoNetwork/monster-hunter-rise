package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	mmpb "npln.nintendo.net/npln-practice/proto/matchmaking/v1"
)

var dashboardOnce sync.Once
var dashboardStart = time.Now()

// startGameDashboard exposes the same /api/stats contract the other Nextendo game servers serve,
// so nextendo-account can poll it for player counts and the dashboard can list rooms.
func startGameDashboard(g *gamesyncServer) {
	addr := os.Getenv("DASH_LISTEN")
	if addr == "" {
		return
	}
	dashboardOnce.Do(func() {
		go func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/api/stats", func(w http.ResponseWriter, r *http.Request) {
				token := os.Getenv("DASH_TOKEN")
				if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(r.URL.Query().Get("key"))) != 1 {
					http.Error(w, "forbidden", 403)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				_ = json.NewEncoder(w).Encode(g.dashboardStats())
			})
			log.Printf("[Rise Dashboard] listening on %s", addr)
			if err := http.ListenAndServe(addr, mux); err != nil {
				log.Printf("[Rise Dashboard] %v", err)
			}
		}()
	})
}

// roomKind labels a room by the mode the host published: Peace Walker searches COOPS for co-op
// ops and VERSUS for Versus Ops, and both must be distinguishable on the dashboard.
// roomKind: Rise keeps a lobby (Lobby4Max_MR...) and, once someone posts a quest, a separate quest
// session (NormalQuest14_MR_V2...) with the same players; label them apart so they don't look doubled.
func roomKind(gs *mmpb.GameSession) string {
	config := gs.GetProperties().GetFields()["_BaseConfigName"].GetStringValue()
	switch {
	case strings.Contains(config, "Quest"):
		return "Quest"
	case strings.HasPrefix(config, "Lobby"):
		return "Lobby"
	case config == "":
		return "Online lobby"
	}
	return config
}

func (g *gamesyncServer) dashboardStats() map[string]any {
	// Gamesync knows who is actually connected; the registry knows each room's shape, including a
	// host who is still waiting on empty slots and so has no second stream yet.
	connected := map[string]bool{}
	g.mu.RLock()
	for id, s := range g.sessions {
		if g.streamCounts[id] > 0 {
			connected[s.UID] = true
		}
	}
	g.mu.RUnlock()

	rooms := []*mmpb.GameSession{}
	if g.registry != nil {
		g.registry.mu.Lock()
		for _, gs := range g.registry.sessions {
			if gs.GetState() == mmpb.GameSession_ACTIVE {
				rooms = append(rooms, proto.Clone(gs).(*mmpb.GameSession))
			}
		}
		g.registry.mu.Unlock()
	}
	sort.Slice(rooms, func(i, j int) bool { return rooms[i].GetName() < rooms[j].GetName() })

	players := []map[string]any{}
	gatherings := []map[string]any{}
	seen := map[uint64]bool{}
	inLobby := 0

	for _, gs := range rooms {
		h := fnv.New32a()
		h.Write([]byte(gs.GetName()))
		id := h.Sum32()

		members := []map[string]any{}
		var hostPid uint64
		var hostName string
		for i, us := range gs.GetUserSessions() {
			pid := pidPourUid(userIDFromPath(us.GetUser()))
			if pid == 0 {
				continue
			}
			name := fmt.Sprintf("Player-%d", pid)
			isHost := i == 0
			if isHost {
				hostPid, hostName = pid, name
			}
			members = append(members, map[string]any{"pid": pid, "name": name, "host": isHost})
			if seen[pid] {
				continue
			}
			seen[pid] = true
			players = append(players, map[string]any{
				"pid": pid, "name": name, "state": "in a lobby", "gathering": id, "isHost": isHost,
				"idleSeconds": 0, "onlineSeconds": int(time.Since(gs.GetCreateTime().AsTime()).Seconds()),
			})
		}
		if len(members) == 0 {
			continue
		}
		inLobby += len(members)

		// The host advertises the slots the room really has (co-op ops run 2-4), so report that
		// rather than a fixed number, otherwise a full room still looks joinable.
		max := int(gs.GetMaxParticipantCount())
		if max < len(members) {
			max = len(members)
		}
		gatherings = append(gatherings, map[string]any{
			"id": id, "hostPid": hostPid, "hostName": hostName, "type": roomKind(gs),
			"state": "active", "count": len(members), "max": max, "players": members,
		})
	}

	// Connected but not in any room: the player is sitting in the online menus.
	uids := make([]string, 0, len(connected))
	for uid := range connected {
		uids = append(uids, uid)
	}
	sort.Strings(uids)
	for _, uid := range uids {
		pid := pidPourUid(uid)
		if pid == 0 || seen[pid] {
			continue
		}
		seen[pid] = true
		players = append(players, map[string]any{
			"pid": pid, "name": fmt.Sprintf("Player-%d", pid), "state": "online",
			"gathering": uint32(0), "isHost": false, "idleSeconds": 0,
		})
	}

	return map[string]any{
		"serverTime":    time.Now().Format("15:04:05"),
		"uptimeSeconds": int(time.Since(dashboardStart).Seconds()),
		"connected":     len(players),
		"inLobby":       inLobby,
		"activeLobbies": len(gatherings),
		"players":       players,
		"gatherings":    gatherings,
		"events":        []any{},
		"methods":       []any{},
		"server": map[string]any{
			"accessKey": nplnTenantID, "stack": "npln", "nexVersion": "NPLN", "sniHost": wonderTLSHostname,
		},
	}
}
