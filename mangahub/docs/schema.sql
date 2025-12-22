-- users table
CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  username TEXT UNIQUE NOT NULL,
  email TEXT UNIQUE NOT NULL,
  password_hash TEXT NOT NULL,
  token_version INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- manga table
CREATE TABLE IF NOT EXISTS manga (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  author TEXT,
  genres TEXT NOT NULL, -- JSON array
  status TEXT,
  total_chapters INTEGER,
  description TEXT,
  cover_url TEXT
);

-- user progress table
CREATE TABLE IF NOT EXISTS user_progress (
  user_id TEXT NOT NULL,
  manga_id TEXT NOT NULL,
  current_chapter INTEGER NOT NULL,
  status TEXT,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, manga_id),
  FOREIGN KEY (user_id) REFERENCES users(id),
  FOREIGN KEY (manga_id) REFERENCES manga(id)
);

-- user progress history table
CREATE TABLE IF NOT EXISTS user_progress_history (
  user_id TEXT NOT NULL,
  manga_id TEXT NOT NULL,
  chapter INTEGER NOT NULL,
  volume INTEGER,
  at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (user_id) REFERENCES users(id),
  FOREIGN KEY (manga_id) REFERENCES manga(id)
);

-- reviews table (bonus feature)
CREATE TABLE IF NOT EXISTS reviews (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id TEXT NOT NULL,
  manga_id TEXT NOT NULL,
  rating INTEGER NOT NULL,
  text TEXT,
  timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (user_id) REFERENCES users(id),
  FOREIGN KEY (manga_id) REFERENCES manga(id)
);
