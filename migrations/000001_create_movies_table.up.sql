CREATE TABLE IF NOT EXISTS movies
(
    id         INT AUTO_INCREMENT PRIMARY KEY,
    created_at TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP,
    title      VARCHAR(50) NOT NULL,
    year       DATETIME    NOT NULL,
    runtime    INT         NOT NULL
);

CREATE TABLE IF NOT EXISTS genres
(
    id   INT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(50) UNIQUE NOT NULL
);

CREATE TABLE IF NOT EXISTS movie_genres
(
    movie_id INT,
    genre_id INT,
    PRIMARY KEY (movie_id, genre_id),
    FOREIGN KEY (movie_id) REFERENCES movies (id) ON DELETE CASCADE,
    FOREIGN KEY (genre_id) REFERENCES genres (id) ON DELETE CASCADE
);
