ALTER TABLE movies
    ADD CONSTRAINT movies_runtime_check CHECK ( runtime >= 0 );
