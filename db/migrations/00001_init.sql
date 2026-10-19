-- +goose Up
CREATE TABLE IF NOT EXISTS sessions (
    id         BIGINT PRIMARY KEY,
    name       VARCHAR(100) NOT NULL,
    start_time TIMESTAMP    NOT NULL,
    end_time   TIMESTAMP,
    count      INTEGER      NOT NULL,
    sf         FLOAT        NOT NULL DEFAULT 0,
    bw         FLOAT        NOT NULL DEFAULT 0,
    tx         FLOAT        NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS packets (
    id             SERIAL PRIMARY KEY,
    measurement_id INTEGER          NOT NULL,
    packet_num     INTEGER          NOT NULL,
    rssi           FLOAT            NOT NULL,
    snrl           DOUBLE PRECISION NOT NULL,
    latitude       FLOAT            NOT NULL,
    longitude      FLOAT            NOT NULL,
    hdop           DOUBLE PRECISION NOT NULL,
    timestamp      VARCHAR(55)      NOT NULL,
    session_id     BIGINT           NOT NULL REFERENCES sessions (id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_packets_session_measurement ON packets (session_id, measurement_id);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_session_count()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NOT EXISTS (
            SELECT 1 FROM packets
            WHERE session_id = NEW.session_id AND measurement_id = NEW.measurement_id AND id <> NEW.id
        ) THEN
            UPDATE sessions SET count = count + 1 WHERE id = NEW.session_id;
        END IF;
        RETURN NEW;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM packets
        WHERE session_id = OLD.session_id AND measurement_id = OLD.measurement_id
    ) THEN
        UPDATE sessions SET count = count - 1 WHERE id = OLD.session_id;
    END IF;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS increment_session_count ON packets;
CREATE TRIGGER increment_session_count
    AFTER INSERT OR DELETE ON packets
    FOR EACH ROW
EXECUTE FUNCTION update_session_count();

UPDATE sessions s
SET count = (SELECT COUNT(DISTINCT measurement_id) FROM packets p WHERE p.session_id = s.id);

-- +goose Down
DROP TRIGGER IF EXISTS increment_session_count ON packets;
DROP FUNCTION IF EXISTS update_session_count();
DROP TABLE IF EXISTS packets;
DROP TABLE IF EXISTS sessions;
