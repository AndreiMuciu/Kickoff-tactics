// twopods is a development check, not part of the game. It plays one ranked
// lobby with eight throwaway managers spread over TWO server instances that
// share a database and Redis, half of them over WebSocket and half by plain
// polling, and verifies that the instances agree on everything.
//
//	go run ./cmd/twopods http://127.0.0.1:8091 http://127.0.0.1:8092
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const origin = "http://localhost:5173"

type user struct {
	name   string
	base   string // the instance this manager talks to
	other  string
	client *http.Client
	ws     bool
}

func (u *user) do(method, base, path string, body any, out any) (int, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, base+path, rd)
	req.Header.Set("Origin", origin)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := u.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	if out != nil {
		_ = json.Unmarshal(data, out)
	}
	return res.StatusCode, nil
}

func (u *user) dial(ctx context.Context) (*websocket.Conn, error) {
	h := http.Header{"Origin": {origin}}
	req, _ := http.NewRequest("GET", u.base+"/api/ws", nil)
	for _, c := range u.client.Jar.Cookies(req.URL) {
		h.Add("Cookie", c.Name+"="+c.Value)
	}
	c, _, err := websocket.Dial(ctx, strings.Replace(u.base, "http", "ws", 1)+"/api/ws", &websocket.DialOptions{HTTPHeader: h})
	if c != nil {
		c.SetReadLimit(8 << 20)
	}
	return c, err
}

type push struct {
	ID     string          `json:"id"`
	Status int             `json:"status"`
	Data   json.RawMessage `json:"data"`
}

type gameView struct {
	Status   string            `json:"status"`
	Round    int               `json:"round"`
	Commit   string            `json:"commit"`
	From     int               `json:"from"`
	Roster   []json.RawMessage `json:"roster"`
	Frames   []json.RawMessage `json:"frames"`
	Place    int               `json:"place"`
	LPChange int               `json:"lpChange"`
	PenRound bool              `json:"penRound"`
	Standing []struct {
		You      bool `json:"you"`
		Patience int  `json:"patience"`
	} `json:"standings"`
}

var failed bool

func check(ok bool, format string, a ...any) {
	mark := "PASS"
	if !ok {
		mark, failed = "FAIL", true
	}
	fmt.Printf("%s  %s\n", mark, fmt.Sprintf(format, a...))
}

func main() {
	if len(os.Args) != 3 {
		fmt.Println("usage: twopods <instance A url> <instance B url>")
		os.Exit(2)
	}
	bases := [2]string{os.Args[1], os.Args[2]}
	var rnd [6]byte
	_, _ = rand.Read(rnd[:])
	tag := hex.EncodeToString(rnd[:3])
	var pw [12]byte
	_, _ = rand.Read(pw[:])
	password := "Tp-" + hex.EncodeToString(pw[:]) // random, never printed

	users := make([]*user, 8)
	for i := range users {
		jar, _ := cookiejar.New(nil)
		u := &user{name: fmt.Sprintf("tp%s_%d", tag, i), base: bases[i%2], other: bases[1-i%2], client: &http.Client{Jar: jar, Timeout: 10 * time.Second}, ws: i < 4}
		code, err := u.do("POST", u.base, "/api/auth/signup", map[string]string{"username": u.name, "email": u.name + "@example.test", "password": password}, nil)
		if err != nil || code != 201 {
			fmt.Println("signup failed:", code, err)
			os.Exit(1)
		}
		// The session cookie was issued by one instance; make the jar send it to the other too.
		a, _ := http.NewRequest("GET", u.base+"/api/me", nil)
		b, _ := http.NewRequest("GET", u.other+"/api/me", nil)
		jar.SetCookies(b.URL, jar.Cookies(a.URL))
		users[i] = u
	}
	code, _ := users[0].do("GET", users[0].other, "/api/me", nil, nil)
	check(code == 200, "a session created on instance A is accepted by instance B (%d)", code)

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	// 1. A change made through instance B reaches a WebSocket held by instance A.
	{
		c, err := users[0].dial(ctx)
		if err != nil {
			fmt.Println("websocket dial failed:", err)
			os.Exit(1)
		}
		_ = c.Write(ctx, websocket.MessageText, []byte(`{"op":"watch","id":"friends","path":"/api/friends"}`))
		_, _, _ = c.Read(ctx) // the first answer
		start := time.Now()
		users[1].do("POST", users[1].base, "/api/friends/request", map[string]string{"username": users[0].name}, nil)
		rctx, rcancel := context.WithTimeout(ctx, 4*time.Second)
		_, data, err := c.Read(rctx)
		rcancel()
		took := time.Since(start)
		check(err == nil && strings.Contains(string(data), users[1].name) && took < 1500*time.Millisecond,
			"friend request sent on B is pushed to a socket on A in %d ms (the watch's own interval is 5000 ms)", took.Milliseconds())
		c.Close(websocket.StatusNormalClosure, "")
	}

	// 2. A socket from a foreign origin, and one without a session, are refused.
	{
		_, res, err := websocket.Dial(ctx, strings.Replace(bases[0], "http", "ws", 1)+"/api/ws", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {"https://evil.example"}}})
		check(err != nil && res != nil && res.StatusCode == 403, "socket from a foreign origin is refused")
		_, res, err = websocket.Dial(ctx, strings.Replace(bases[0], "http", "ws", 1)+"/api/ws", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {origin}}})
		check(err != nil && res != nil && res.StatusCode == 401, "socket without a session is refused")
	}

	if os.Getenv("QUICK") != "" { // skip the lobby, which takes minutes
		limits(users[0], bases, tag)
		if failed {
			os.Exit(1)
		}
		return
	}

	// 3. Eight managers queue for ranked through both instances and land in one lobby.
	for _, u := range users {
		u.do("POST", u.base, "/api/ranked/queue", map[string]string{"mode": "ranked"}, nil)
	}
	lobby := map[string]bool{}
	ids := make([]string, len(users))
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		found := 0
		for i, u := range users {
			var v struct {
				LobbyID string `json:"lobbyId"`
			}
			u.do("GET", u.base, "/api/ranked", nil, &v)
			if v.LobbyID != "" {
				ids[i], lobby[v.LobbyID] = v.LobbyID, true
				found++
			}
		}
		if found == len(users) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	check(len(lobby) == 1 && ids[7] != "", "all eight, queued through two instances, are in the same lobby (%d lobby)", len(lobby))
	if len(lobby) != 1 {
		os.Exit(1)
	}
	id := ids[0]

	// 4. Everyone follows the lobby to the end: four by WebSocket, four by polling.
	type result struct {
		place, change int
		rounds        map[int]string // round -> hash of all frames received
		gaps          int
		per           map[int][]string // round -> hash of each frame
		commit        map[int]string   // round -> the match's public commitment, shared by both sides
		warm          map[int]bool     // round -> it was announced as a warm-up
		patience      map[int]int      // round -> own patience as that round began
	}
	results := make([]result, len(users))
	var wg sync.WaitGroup
	for i, u := range users {
		wg.Add(1)
		go func(i int, u *user) {
			defer wg.Done()
			r := result{rounds: map[int]string{}, per: map[int][]string{}, commit: map[int]string{}, warm: map[int]bool{}, patience: map[int]int{}}
			round, frames := 0, 0
			h := sha256.New()
			apply := func(v gameView) bool {
				if v.Round != round {
					if round > 0 && frames > 0 {
						r.rounds[round] = fmt.Sprintf("%d:%s", frames, hex.EncodeToString(h.Sum(nil))[:12])
					}
					round, frames = v.Round, 0
					h = sha256.New()
				}
				for _, st := range v.Standing {
					if _, seen := r.patience[v.Round]; st.You && !seen {
						r.patience[v.Round] = st.Patience
					}
				}
				if v.Round > 0 {
					r.warm[v.Round] = r.warm[v.Round] || v.PenRound
				}
				if len(v.Roster) > 0 {
					r.commit[v.Round] = v.Commit
					if v.From == frames {
						for _, f := range v.Frames {
							h.Write(f)
							one := sha256.Sum256(f)
							r.per[v.Round] = append(r.per[v.Round], hex.EncodeToString(one[:6]))
						}
						frames += len(v.Frames)
					} else if v.From > frames {
						r.gaps++
					}
				}
				if v.Status == "done" {
					if frames > 0 {
						r.rounds[round] = fmt.Sprintf("%d:%s", frames, hex.EncodeToString(h.Sum(nil))[:12])
					}
					r.place, r.change = v.Place, v.LPChange
					return true
				}
				return false
			}
			if u.ws {
				c, err := u.dial(ctx)
				if err != nil {
					return
				}
				defer c.CloseNow()
				_ = c.Write(ctx, websocket.MessageText, []byte(`{"op":"watch","id":"game","path":"/api/lobby?id=`+id+`","round":0,"from":0}`))
				for {
					_, data, err := c.Read(ctx)
					if err != nil {
						return
					}
					var p push
					var v gameView
					if json.Unmarshal(data, &p) != nil || p.Status != 200 || json.Unmarshal(p.Data, &v) != nil {
						continue
					}
					if apply(v) {
						break
					}
				}
			} else {
				for ctx.Err() == nil {
					var v gameView
					code, _ := u.do("GET", u.base, fmt.Sprintf("/api/lobby?id=%s&from=%d", id, frames), nil, &v)
					if code == 200 && apply(v) {
						break
					}
					time.Sleep(time.Second)
				}
			}
			results[i] = r
		}(i, u)
	}
	wg.Wait()

	places := []int{}
	gaps := 0
	for _, r := range results {
		places = append(places, r.place)
		gaps += r.gaps
	}
	sort.Ints(places)
	// The first two rounds are penalty rounds between the managers: half of
	// them lose each one, and must not pay for it.
	warm, kept, paid := true, true, false
	for _, r := range results {
		warm = warm && r.warm[1] && r.warm[2] && !r.warm[3]
		kept = kept && r.patience[1] == 100 && r.patience[2] == 100 && r.patience[3] == 100
		paid = paid || (r.patience[4] > 0 && r.patience[4] < 100)
	}
	check(warm, "rounds 1 and 2 are penalty rounds for everyone, round 3 is a match")
	check(kept, "nobody lost patience in the penalty rounds: all at 100 when round 3 begins")
	check(paid, "from round 3 a defeat costs patience")
	check(fmt.Sprint(places) == "[1 2 3 4 5 6 7 8]", "both instances agree on the final places: %v", places)
	check(gaps == 0, "no manager ever skipped match frames (%d gaps)", gaps)

	// The two managers of a match sit on different instances half the time; they
	// must have received the very same frames. Count hashes per round.
	odd, matches, short := 0, 0, 0
	for round := 1; round <= 14; round++ {
		sides := map[string][]int{} // match (its public commitment) -> the managers who watched it
		for i, r := range results {
			if c := r.commit[round]; c != "" {
				sides[c] = append(sides[c], i)
			}
		}
		for _, who := range sides {
			if len(who) != 2 { // a ghost match has a single viewer
				continue
			}
			matches++
			a, b := results[who[0]].per[round], results[who[1]].per[round]
			if len(a) != len(b) {
				short++ // one of them was not served the last frames before the round moved on
			}
			for k := 0; k < min(len(a), len(b)); k++ {
				if a[k] != b[k] {
					odd++
					fmt.Printf("      round %d  %s and %s differ at frame %d\n", round, users[who[0]].name, users[who[1]].name, k)
					break
				}
			}
		}
	}
	check(odd == 0 && matches > 0, "opponents received identical frames, whichever instance or transport served them (%d matches compared frame by frame)", matches)
	// At high GAME_SPEED the result phase is shorter than a client's one-second
	// read, so the last frames can be missed; at the real speed it lasts 9 s.
	fmt.Printf("INFO  %d of %d matches ended before one side had read the last frames\n", short, matches)

	// 5. Points were written exactly once, and both instances report the same.
	once, same := true, true
	for i, u := range users {
		var a, b struct {
			Rating int `json:"rating"`
			Games  int `json:"games"`
		}
		u.do("GET", u.base, "/api/ranked", nil, &a)
		u.do("GET", u.other, "/api/ranked", nil, &b)
		want := max(0, results[i].change)
		once = once && a.Games == 1 && a.Rating == want
		same = same && a == b
	}
	check(once, "every manager was rated exactly once, with the points of his place")
	check(same, "both instances report the same points")

	limits(users[0], bases, tag)
	if failed {
		os.Exit(1)
	}
	fmt.Println("all checks passed")
}

// limits checks that a limit is one counter for all instances. An account
// allows 8 login attempts in a burst. Wrong passwords are alternated between
// the two instances: a common counter stops the 9th, a counter per instance
// would let all 14 through (7 each).
func limits(u *user, bases [2]string, tag string) {
	// A manager nobody has tried to log in as yet, so the bucket is full. The
	// per-IP limit (10 a minute, already used by the sign-ups) must not interfere.
	time.Sleep(61 * time.Second)
	passed := 0
	for i := 0; i < 14; i++ {
		code, _ := u.do("POST", bases[i%2], "/api/auth/login", map[string]string{"username": u.name, "password": "wrong-" + tag}, nil)
		if code == 429 {
			break
		}
		passed++
	}
	check(passed == 8, "wrong passwords alternated between A and B: %d passed before the block (8 = one common counter, 14 = one per instance)", passed)
}
