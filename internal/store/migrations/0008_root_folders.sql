-- 0008: root-folder check results (writer: rootfolders).

ALTER TABLE root_folders ADD COLUMN writable INTEGER NOT NULL DEFAULT 0;
ALTER TABLE root_folders ADD COLUMN problem TEXT NOT NULL DEFAULT '';
