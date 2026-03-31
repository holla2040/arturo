# Dedicated Controller Machine Setup

The dev desktop (192.168.0.3) currently runs the controller, terminal, and Redis. This checklist covers migrating to a dedicated Ubuntu machine.

## Setup Checklist

- [ ] Install and configure Redis (with ACL from `redis/redis-acl.conf`)
- [ ] Enable chrony as LAN NTP server (`allow 192.168.0.0/24` in `/etc/chrony/chrony.conf`, then `sudo systemctl restart chrony`)
- [ ] Build and deploy controller binary (`cd subsystems && go build -o controller ./cmd/controller`)
- [ ] Build and deploy terminal binary (`cd subsystems && go build -o terminal ./cmd/terminal`)
- [ ] Update station `.env` files with new controller IP (`REDIS_HOST`, which also sets `NTP_SERVER`)
- [ ] Verify station NTP sync from the new machine (serial log: `NTP: Time synced: ...`)
- [ ] Set up supervisor (`tools/supervisor/`) for process management
- [ ] Configure SQLite database location and backup strategy
- [ ] Set machine timezone to America/Denver
