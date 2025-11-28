CREATE TABLE IF NOT EXISTS users
(
    id            VARCHAR(36) PRIMARY KEY,
    created_at    TIMESTAMP          NOT NULL DEFAULT CURRENT_TIMESTAMP,
    name          VARCHAR(50)        NOT NULL,
    email         VARCHAR(50) UNIQUE NOT NULL,
    password_hash VARCHAR(255)       NOT NULL,
    activated     BOOL               NOT NULL
);
