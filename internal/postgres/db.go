package postgres

import (
	"Lora_Esp_Gsm_Gps_project/configs"
	migrations "Lora_Esp_Gsm_Gps_project/db"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"Lora_Esp_Gsm_Gps_project/internal/models"
)

var ErrNotFound = errors.New("not found")

type Repo struct {
	db *sql.DB
}

func NewPostgresRepo(db *sql.DB) *Repo {
	return &Repo{
		db: db,
	}
}

func NewPostgresDB(config configs.Config) (*sql.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s",
		config.DbHost, config.DbPort, config.DbUser, config.DbPassword, config.DbName)

	var err error
	var db *sql.DB
	for i := 0; i < 5; i++ {
		db, err = sql.Open("pgx", dsn)
		if err != nil {
			log.Printf("Failed to open database (attempt %d): %v", i+1, err)
			time.Sleep(2 * time.Second)
			continue
		}

		err = db.Ping()
		if err != nil {
			log.Printf("Failed to ping database (attempt %d): %v", i+1, err)
			db.Close()
			time.Sleep(2 * time.Second)
			continue
		}
		break
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to database after 5 attempts: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(10 * time.Minute)

	log.Println("Successfully connected to PostgreSQL!")
	return db, nil
}

func Migrate(ctx context.Context, db *sql.DB) error {
	goose.SetBaseFS(migrations.Migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}
	return nil
}

func (r *Repo) Save(ctx context.Context, packet *models.Packet, sessionId int) error {
	if sessionId == 0 {
		return errors.New("failed to save packet: session is not set (START_MEASUREMENT not received)")
	}
	_, err := r.db.ExecContext(ctx, "INSERT INTO PACKETS "+
		"(id, measurement_id, packet_num, rssi, snrl, latitude, longitude, hdop, timestamp, session_id) values (DEFAULT,$1, $2, $3, $4, $5, $6, $7, $8, $9)",
		packet.MeasurementId,
		packet.PacketNum,
		packet.RSSI,
		packet.SNRL,
		packet.Coordinate.Latitude,
		packet.Coordinate.Longitude,
		packet.Hdop,
		packet.Timestamp.UTC().Format(models.TimeLayout),
		sessionId,
	)
	if err != nil {
		return fmt.Errorf("failed to save packet: %w", err)
	}
	return err
}

func (r *Repo) SaveSession(ctx context.Context, session models.Session) error {
	_, err := r.db.ExecContext(ctx, "INSERT INTO SESSIONS "+
		"(id, name, start_time, end_time, count, sf, bw, tx) values ($1, $2, $3, $4, $5, $6, $7, $8)",
		session.Id,
		session.Name,
		session.StartTime,
		session.EndTime,
		session.Count,
		session.Sf,
		session.Bandwidth,
		session.Tx,
	)

	if err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}
	return err
}

func (r *Repo) RemoveSession(ctx context.Context, sessionId int) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", sessionId)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	return nil
}

func (r *Repo) GetAllSessions(ctx context.Context) ([]models.Session, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, name, start_time, end_time, count, sf, bw, tx "+
		"FROM sessions")
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions: %w", err)
	}
	defer rows.Close()

	sessions := []models.Session{}
	for rows.Next() {
		var session models.Session
		var endTime sql.NullString
		err := rows.Scan(
			&session.Id,
			&session.Name,
			&session.StartTime,
			&endTime,
			&session.Count,
			&session.Sf,
			&session.Bandwidth,
			&session.Tx,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}
		session.EndTime = endTime.String
		sessions = append(sessions, session)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over sessions: %w", err)
	}

	return sessions, nil
}

func (r *Repo) FindBySession(ctx context.Context, sessionId int) ([]models.Packet, error) {
	rows, err := r.db.QueryContext(ctx, `
        SELECT 
            measurement_id,
            AVG(rssi) as avg_rssi,
            AVG(snrl) as avg_snrl,
            AVG(latitude) as avg_latitude,
            AVG(longitude) as avg_longitude,
            AVG(hdop) as avg_hdop,
            MAX(timestamp) as last_timestamp
        FROM packets 
        WHERE session_id = $1
        GROUP BY measurement_id
        ORDER BY measurement_id
    `, sessionId)

	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var packets []models.Packet
	for rows.Next() {
		var packet models.Packet
		var timestamp string
		err := rows.Scan(
			&packet.MeasurementId,
			&packet.RSSI,
			&packet.SNRL,
			&packet.Coordinate.Latitude,
			&packet.Coordinate.Longitude,
			&packet.Hdop,
			&timestamp,
		)
		if err != nil {
			return nil, err
		}
		if packet.Timestamp, err = time.Parse(models.TimeLayout, timestamp); err != nil {
			log.Printf("Unexpected packet timestamp %q: %v", timestamp, err)
		}
		packets = append(packets, packet)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(packets) == 0 {
		return nil, ErrNotFound
	}

	return packets, nil
}

func (r *Repo) FindLastSessionId(ctx context.Context) (int, error) {
	var lastSessionId int
	err := r.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0)"+
		" FROM sessions").Scan(&lastSessionId)
	if err != nil {
		return 0, fmt.Errorf("failed to get last session ID: %w", err)
	}
	return lastSessionId, nil
}

func (r *Repo) AddSettingsToSession(ctx context.Context, sessionId int, sf, tx, bw float32) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE sessions
		SET sf = $2, tx = $3, bw = $4
		WHERE id = $1
	`, sessionId, sf, tx, bw)
	if err != nil {
		return fmt.Errorf("failed to update session settings: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("session with id %d not found", sessionId)
	}

	return nil
}

func (r *Repo) CheckSettings(ctx context.Context, sessionId int) (missing bool, hasPackets bool, err error) {
	err = r.db.QueryRowContext(ctx, `
        SELECT (sf = 0 OR bw = 0 OR tx = 0),
               EXISTS (SELECT 1 FROM packets WHERE session_id = $1)
        FROM sessions
        WHERE id = $1
    `, sessionId).Scan(&missing, &hasPackets)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, fmt.Errorf("session %d: %w", sessionId, ErrNotFound)
	}
	if err != nil {
		return false, false, fmt.Errorf("failed to check settings: %w", err)
	}
	return missing, hasPackets, nil
}
