-- 0111: upload speed and share ratio of a download (writer: queue), recorded
-- from each client snapshot so queue reads show them without a live call.

ALTER TABLE downloads ADD COLUMN up_speed INTEGER NOT NULL DEFAULT 0;
ALTER TABLE downloads ADD COLUMN ratio REAL NOT NULL DEFAULT 0;
