CREATE TABLE urls (
	url TEXT NOT NULL,
	hash TEXT NOT NULL,
	algo TEXT NOT NULL,
	PRIMARY KEY (url, algo)
);
