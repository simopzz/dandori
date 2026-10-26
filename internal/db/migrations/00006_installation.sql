-- +goose Up
-- The single installation of the app: the id column only ever holds true.
-- Its Content Language is recorded on first start and never changes.
CREATE TABLE installation (
    id               boolean PRIMARY KEY DEFAULT true CHECK (id),
    content_language text    NOT NULL CHECK (content_language IN ('it', 'en'))
);

-- +goose Down
DROP TABLE installation;
