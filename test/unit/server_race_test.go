package unit

import (
	"Lora_Esp_Gsm_Gps_project/cmd/server"
	"Lora_Esp_Gsm_Gps_project/internal/esp"
	"Lora_Esp_Gsm_Gps_project/internal/models"
	"Lora_Esp_Gsm_Gps_project/internal/service"
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

var errFakeNotFound = errors.New("not found")

type fakeSettings struct{ sf, tx, bw float32 }

type fakeRepo struct {
	mu               sync.Mutex
	sessions         []models.Session
	settings         map[int]fakeSettings
	packets          []models.Packet
	packetsBySession map[int]int
}

func newFakeRepo(sessions ...models.Session) *fakeRepo {
	return &fakeRepo{
		sessions:         sessions,
		settings:         make(map[int]fakeSettings),
		packetsBySession: make(map[int]int),
	}
}

func (r *fakeRepo) sessionExists(id int) bool {
	for _, s := range r.sessions {
		if s.Id == id {
			return true
		}
	}
	return false
}

func (r *fakeRepo) lastSessionId() int {
	last := 0
	for _, s := range r.sessions {
		if s.Id > last {
			last = s.Id
		}
	}
	return last
}

func (r *fakeRepo) Save(_ context.Context, p *models.Packet, sessionId int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if sessionId == 0 {
		sessionId = r.lastSessionId()
	}
	r.packets = append(r.packets, *p)
	r.packetsBySession[sessionId]++
	return nil
}

func (r *fakeRepo) SaveSession(_ context.Context, s models.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions = append(r.sessions, s)
	return nil
}

func (r *fakeRepo) RemoveSession(_ context.Context, id int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, s := range r.sessions {
		if s.Id == id {
			r.sessions = append(r.sessions[:i], r.sessions[i+1:]...)
			delete(r.settings, id)
			delete(r.packetsBySession, id)
			return nil
		}
	}
	return nil
}

func (r *fakeRepo) GetAllSessions(context.Context) ([]models.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]models.Session(nil), r.sessions...), nil
}

func (r *fakeRepo) FindBySession(context.Context, int) ([]models.Packet, error) { return nil, nil }

func (r *fakeRepo) FindLastSessionId(context.Context) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastSessionId(), nil
}

func (r *fakeRepo) AddSettingsToSession(_ context.Context, sessionId int, sf, tx, bw float32) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.sessionExists(sessionId) {
		return fmt.Errorf("session %d: %w", sessionId, errFakeNotFound)
	}
	r.settings[sessionId] = fakeSettings{sf: sf, tx: tx, bw: bw}
	return nil
}

func (r *fakeRepo) CheckSettings(_ context.Context, sessionId int) (missing bool, hasPackets bool, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.sessionExists(sessionId) {
		return false, false, fmt.Errorf("session %d: %w", sessionId, errFakeNotFound)
	}
	st := r.settings[sessionId] // для отсутствующей записи — нули, как DEFAULT 0 в БД
	missing = st.sf == 0 || st.tx == 0 || st.bw == 0
	hasPackets = r.packetsBySession[sessionId] > 0
	return missing, hasPackets, nil
}

func (r *fakeRepo) packetCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.packets)
}

var _ service.Repository = (*fakeRepo)(nil)

type device struct {
	t    *testing.T
	conn net.Conn
	r    *bufio.Reader
}

func dial(t *testing.T, addr string) *device {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return &device{t: t, conn: conn, r: bufio.NewReader(conn)}
}

func (d *device) send(line string) {
	if _, err := d.conn.Write([]byte(line + "\n")); err != nil {
		d.t.Errorf("send %q: %v", line, err)
	}
}

func (d *device) waitFor(prefix string, timeout time.Duration) string {
	d.t.Helper()
	_ = d.conn.SetReadDeadline(time.Now().Add(timeout))
	for {
		line, err := d.r.ReadString('\n')
		if err != nil {
			d.t.Fatalf("ждали строку %q, получили ошибку: %v", prefix, err)
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
}

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("не дождались: %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func startServer(t *testing.T, repo *fakeRepo) (string, *esp.ESPConnector) {
	t.Helper()
	connector := esp.NewESPConnector()
	svc := service.NewDataService(connector)
	svc.Repo = repo
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	svc.StartProcessing(ctx)

	srv := server.NewServer(svc, connector)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { ln.Close() })
	return ln.Addr().String(), connector
}

func TestEspAndClientsConcurrently(t *testing.T) {
	repo := newFakeRepo(
		models.Session{Id: 1, Name: "test", StartTime: "2025-01-01T10:00:00"},
	)
	addr, _ := startServer(t, repo)

	espDev := dial(t, addr)
	clients := make([]*device, 5)
	for i := range clients {
		clients[i] = dial(t, addr)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		espDev.send("IDENTIFY")
		for i := 0; i < 20; i++ {
			espDev.send("MASTER_STATUS ISALIVE")
			espDev.send("SLAVE_STATUS ISALIVE")
		}
	}()
	for _, c := range clients {
		wg.Add(1)
		go func(c *device) {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				c.send("GET_MEASUREMENT_SESSIONS")
			}
		}(c)
	}
	wg.Wait()

	for _, c := range clients {
		line := c.waitFor("SESSIONS:", 3*time.Second)
		if !strings.Contains(line, "test") {
			t.Errorf("в списке сессий нет тестовой сессии: %q", line)
		}
	}
}

func TestMeasurementFlow(t *testing.T) {
	repo := newFakeRepo()
	addr, connector := startServer(t, repo)

	espDev := dial(t, addr)
	ui := dial(t, addr)

	espDev.send("IDENTIFY")
	waitUntil(t, 2*time.Second, connector.IsConnected, "ESP зарегистрирован")

	ui.send("SET_SETTINGS: SF=7, TX=14, BW=125")
	espDev.waitFor("SET_SETTINGS:", 2*time.Second)

	ui.send("START_MEASUREMENT: 1")
	espDev.waitFor("START_MEASUREMENT", 2*time.Second)

	for i := 1; i <= 10; i++ {
		espDev.send(`MEASUREMENT: {"id":1,"packet_number":` + itoa(i) +
			`,"rssi":-70,"snr":7.5,"coordinate":{"latitude":"59.9","longitude":"30.3"},"hdop":0.9}`)
	}

	line := ui.waitFor("MEASUREMENT:", 3*time.Second)
	if !strings.Contains(line, "-70.000000") {
		t.Errorf("ожидали среднее RSSI -70, получили: %q", line)
	}
	waitUntil(t, 2*time.Second, func() bool { return repo.packetCount() == 10 }, "10 пакетов сохранено")
}

func itoa(i int) string { return strconv.Itoa(i) }

func TestPendingMessagesSentOnce(t *testing.T) {
	addr, connector := startServer(t, newFakeRepo())

	ui := dial(t, addr)
	ui.send("START_MEASUREMENT: 1")
	ui.send("SET_SETTINGS: SF=7, TX=14, BW=125")
	time.Sleep(50 * time.Millisecond)

	espDev := dial(t, addr)
	espDev.send("IDENTIFY")
	waitUntil(t, 2*time.Second, connector.IsConnected, "ESP зарегистрирован")

	espDev.waitFor("SET_SETTINGS:", 12*time.Second)
	espDev.waitFor("START_MEASUREMENT SESSION_ID=1", 2*time.Second)
	if got := connector.GetCurrentSession(); got != 1 {
		t.Errorf("CurrentSession = %d, want 1", got)
	}

	_ = espDev.conn.SetReadDeadline(time.Now().Add(11 * time.Second))
	if line, err := espDev.r.ReadString('\n'); err == nil {
		t.Errorf("pending переотправлен: %q", line)
	}
}

func TestEspDisconnectDetected(t *testing.T) {
	addr, connector := startServer(t, newFakeRepo())

	espDev := dial(t, addr)
	espDev.send("IDENTIFY")
	waitUntil(t, 2*time.Second, connector.IsConnected, "ESP зарегистрирован")

	espDev.conn.Close()
	waitUntil(t, 2*time.Second, func() bool { return !connector.IsConnected() }, "ESP помечен отключённым")
}

func TestClientGetsParseError(t *testing.T) {
	addr, _ := startServer(t, newFakeRepo())
	ui := dial(t, addr)
	ui.send("START_MEASUREMENT: abc")
	ui.waitFor("ERROR:", 2*time.Second)
}
