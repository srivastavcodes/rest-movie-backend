CREATE TABLE IF NOT EXISTS tokens
(
    hash    BINARY(32)  NOT NULL,
    user_id VARCHAR(36) NOT NULL,
    expiry  TIMESTAMP   NOT NULL,
    scope   VARCHAR(32) NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
