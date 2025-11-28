CREATE TABLE IF NOT EXISTS permissions
(
    id   INT AUTO_INCREMENT PRIMARY KEY,
    code VARCHAR(50) NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS user_permissions
(
    user_id       VARCHAR(36) NOT NULL,
    permission_id INT         NOT NULL,
    PRIMARY KEY (user_id, permission_id),
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    FOREIGN KEY (permission_id) REFERENCES permissions (id) ON DELETE CASCADE
);

INSERT INTO permissions (code)
VALUES ('user'),
       ('admin'),
       ('moderator');
