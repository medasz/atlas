-- 端口状态列：承载多模式 TCP 扫描（SYN/connect/ACK/FIN/Null/Xmas）产出的端口状态。
-- 状态词表：open|closed|filtered|timeout|open|filtered|unfiltered
ALTER TABLE ports ADD COLUMN IF NOT EXISTS state varchar(16) NOT NULL DEFAULT 'open';
