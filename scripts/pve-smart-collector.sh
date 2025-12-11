#!/bin/bash
# pve-smart-collector.sh
# Collects SMART data from all physical disks and writes to JSON file
# Run as root via cron every 5 minutes (SMART data doesn't change frequently)
#
# Usage: /usr/local/bin/pve-smart-collector.sh
# Cron:  */5 * * * * root /usr/local/bin/pve-smart-collector.sh

OUTPUT_DIR="/var/lib/pve-exporter"
OUTPUT_FILE="${OUTPUT_DIR}/smart.json"
TEMP_FILE="${OUTPUT_DIR}/smart.json.tmp"
LOCK_FILE="${OUTPUT_DIR}/.smart-collector.lock"
HOSTNAME=$(hostname -f 2>/dev/null || hostname)

# Ensure output directory exists before anything else
mkdir -p "$OUTPUT_DIR" || exit 1

# Use lock file to prevent parallel execution
exec 200>"$LOCK_FILE"
if ! flock -n 200; then
    exit 0
fi

echo "{\"hostname\":\"$HOSTNAME\",\"timestamp\":$(date +%s),\"disks\":[" > "$TEMP_FILE"

first=true
for disk in $(lsblk -d -n -o NAME,TYPE | awk '$2=="disk" && $1!~/^zd/ && $1!~/^loop/ {print $1}'); do
    smart_json=$(/usr/sbin/smartctl -j -a "/dev/$disk" 2>/dev/null)
    [ -z "$smart_json" ] && continue
    
    parsed=$(echo "$smart_json" | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    if 'device' not in data: sys.exit(1)
    r = {'device': '$disk', 'model': data.get('model_name', 'Unknown'), 'serial': data.get('serial_number', 'Unknown'), 'type': 'unknown', 'healthy': 1}
    di = data.get('device', {})
    if di.get('protocol') == 'NVMe': r['type'] = 'nvme'
    elif di.get('protocol') == 'ATA': r['type'] = 'sata'
    ss = data.get('smart_status', {})
    if 'passed' in ss: r['healthy'] = 1 if ss['passed'] else 0
    nv = data.get('nvme_smart_health_information_log', {})
    if nv:
        r['temperature'] = nv.get('temperature')
        r['power_on_hours'] = nv.get('power_on_hours')
        r['percentage_used'] = nv.get('percentage_used')
        if 'data_units_written' in nv: r['data_written_bytes'] = nv['data_units_written'] * 512000
        if 'available_spare' in nv: r['available_spare_percent'] = nv['available_spare']
    pt = data.get('power_on_time', {})
    if 'hours' in pt: r['power_on_hours'] = pt['hours']
    ti = data.get('temperature', {})
    if 'current' in ti and 'temperature' not in r: r['temperature'] = ti['current']
    for a in data.get('ata_smart_attributes', {}).get('table', []):
        if a.get('id') == 194 and 'temperature' not in r: r['temperature'] = a.get('raw', {}).get('value')
    r = {k: v for k, v in r.items() if v is not None}
    print(json.dumps(r))
except: sys.exit(1)
" 2>/dev/null)
    
    if [ -n "$parsed" ]; then
        [ "$first" = true ] && first=false || echo "," >> "$TEMP_FILE"
        echo "$parsed" >> "$TEMP_FILE"
    fi
done

echo "]}" >> "$TEMP_FILE"

# Only move if temp file exists and is valid
if [ -f "$TEMP_FILE" ]; then
    mv "$TEMP_FILE" "$OUTPUT_FILE"
    chmod 644 "$OUTPUT_FILE"
fi
