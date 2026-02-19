/* =====================================================================
   Arturo Terminal Application
   ===================================================================== */
var App = (function() {
    'use strict';

    // =================================================================
    // State
    // =================================================================
    var state = {
        employee: null,       // {id, name}
        stations: {},         // instance -> registry data
        stationStates: {},    // instance -> {state, test_run_id, ...}
        stationTemps: {},     // instance -> {first_stage, second_stage}
        stationPumpStatus: {}, // instance -> {pump_on, at_temp, regen, status_1, ...}
        estop: { active: false },
        wsConnected: false,
        currentView: 'login',
        detailStation: null,  // currently viewing station
        detailRMA: null,      // currently viewing RMA ID
        startTestStation: null,
        startTestRMAId: null,
        tempChartData: { timestamps: [], first: [], second: [] }
    };

    var MAX_CHART_POINTS = 720; // 1 hour at 5s intervals

    // =================================================================
    // Utilities
    // =================================================================
    function escapeHtml(s) {
        if (s == null) return '';
        var div = document.createElement('div');
        div.appendChild(document.createTextNode(String(s)));
        return div.innerHTML;
    }

    function regenDescription(ch) {
        if (!ch) return '';
        switch (ch) {
            case 'A': case '\\': return 'Pump OFF';
            case 'B': case 'C': case 'E': case '^': case ']':
            case 'l': case 'm': case '_': case 'r': case 's':
            case 't': case 'u': case 'v': case "'":
                return 'Warmup';
            case 'D': case 'F': case 'G': case 'Q': case 'R':
                return 'Purge gas failure';
            case 'H': return 'Extended purge';
            case 'S': return 'Repurge cycle';
            case 'I': case 'J': case 'K': case 'T':
            case 'a': case 'b': case 'j': case 'n':
                return 'Rough to base pressure';
            case 'L': return 'Rate of rise';
            case 'M': case 'N': case 'c': case 'd': case 'o':
                return 'Cooldown';
            case 'P': return 'Regen complete';
            case 'U': return 'FastRegen start';
            case 'V': return 'Regen aborted';
            case 'W': return 'Delay restart';
            case 'X': case 'Y': return 'Power failure';
            case 'Z': return 'Delay start';
            case 'O': case '[': return 'Zeroing TC gauge';
            case 'f': return 'Share regen wait';
            case 'e': return 'Repurge (FastRegen)';
            case 'h': return 'Purge coordinate wait';
            case 'i': return 'Rough coordinate wait';
            case 'k': return 'Purge gas fail recovery';
            default: return 'Unknown (' + ch + ')';
        }
    }

    function formatUptime(secs) {
        if (secs == null || secs <= 0) return '--';
        var d = Math.floor(secs / 86400);
        var h = Math.floor((secs % 86400) / 3600);
        var m = Math.floor((secs % 3600) / 60);
        var s = secs % 60;
        if (d > 0) return d + 'd ' + h + 'h';
        if (h > 0) return h + 'h ' + m + 'm';
        if (m > 0) return m + 'm ' + s + 's';
        return s + 's';
    }

    function formatTime(iso) {
        if (!iso) return '--';
        try {
            var d = new Date(iso);
            if (isNaN(d.getTime())) return '--';
            return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
        } catch(e) { return '--'; }
    }

    function formatDateTime(iso) {
        if (!iso) return '--';
        try {
            var d = new Date(iso);
            if (isNaN(d.getTime())) return '--';
            return d.toLocaleString([], { month:'short', day:'numeric', hour:'2-digit', minute:'2-digit' });
        } catch(e) { return '--'; }
    }

    function elapsed(startISO) {
        if (!startISO) return '--';
        try {
            var secs = Math.floor((Date.now() - new Date(startISO).getTime()) / 1000);
            return secs >= 0 ? formatUptime(secs) : '--';
        } catch(e) { return '--'; }
    }

    function formatTemp(k) {
        if (k == null) return '--';
        return k > 99.9 ? Math.round(k).toString() : k.toFixed(1);
    }

    function formatBytes(b) {
        if (b == null) return '--';
        if (b >= 1048576) return (b / 1048576).toFixed(1) + ' MB';
        if (b >= 1024) return (b / 1024).toFixed(1) + ' KB';
        return b + ' B';
    }

    // =================================================================
    // API
    // =================================================================
    function api(method, url, body, cb) {
        var xhr = new XMLHttpRequest();
        xhr.open(method, url, true);
        xhr.setRequestHeader('Accept', 'application/json');
        if (state.employee) {
            xhr.setRequestHeader('X-Employee-ID', state.employee.id);
        }
        if (body) {
            xhr.setRequestHeader('Content-Type', 'application/json');
        }
        xhr.onreadystatechange = function() {
            if (xhr.readyState === 4) {
                if (xhr.status >= 200 && xhr.status < 300) {
                    try {
                        var data = xhr.responseText ? JSON.parse(xhr.responseText) : null;
                        cb(null, data);
                    } catch(e) { cb(null, null); }
                } else {
                    var errMsg = 'HTTP ' + xhr.status;
                    try {
                        var ed = JSON.parse(xhr.responseText);
                        if (ed.error) errMsg = ed.error;
                    } catch(e) {}
                    cb(new Error(errMsg));
                }
            }
        };
        xhr.send(body ? JSON.stringify(body) : null);
    }

    // =================================================================
    // View Management
    // =================================================================
    function showView(name) {
        state.currentView = name;
        var views = document.querySelectorAll('.view');
        for (var i = 0; i < views.length; i++) views[i].classList.remove('active');
        var el = document.getElementById('view-' + name);
        if (el) el.classList.add('active');

        var hdr = document.getElementById('app-header');
        hdr.style.display = (name === 'login') ? 'none' : 'flex';

        if (name === 'stations') {
            loadStations();
        } else if (name === 'station-detail' && state.detailStation) {
            loadStationDetail(state.detailStation);
        } else if (name === 'rma-list') {
            loadRMAs();
        } else if (name === 'rma-detail' && state.detailRMA) {
            loadRMADetail(state.detailRMA);
        }
    }

    // =================================================================
    // Login
    // =================================================================
    function login() {
        var empId = document.getElementById('login-employee-id').value.trim();
        var name = document.getElementById('login-name').value.trim();
        if (!empId || !name) {
            showError('login-error', 'Employee ID and Name are required');
            return;
        }
        api('POST', '/auth/login', { employee_id: empId, name: name }, function(err, data) {
            if (err) {
                showError('login-error', err.message);
                return;
            }
            state.employee = { id: empId, name: name };
            document.getElementById('employee-name').textContent = name;
            showView('stations');
            connectWebSocket();
        });
    }

    function logout() {
        state.employee = null;
        showView('login');
    }

    function showError(id, msg) {
        var el = document.getElementById(id);
        if (el) {
            el.textContent = msg;
            el.style.display = msg ? 'block' : 'none';
        }
    }

    // =================================================================
    // Stations
    // =================================================================
    function loadStations() {
        api('GET', '/stations', null, function(err, data) {
            if (!err && Array.isArray(data)) {
                state.stations = {};
                for (var i = 0; i < data.length; i++) {
                    state.stations[data[i].Instance] = data[i];
                }
            }
            renderStationGrid();
        });
    }

    function getStationState(instance) {
        return state.stationStates[instance] || {};
    }

    function renderStationGrid() {
        var grid = document.getElementById('station-grid');
        var keys = Object.keys(state.stations).sort();
        document.getElementById('stat-stations').textContent = keys.length;

        if (keys.length === 0) {
            grid.innerHTML = '<div class="empty-state">No stations reporting</div>';
            return;
        }

        var html = '';
        for (var i = 0; i < keys.length; i++) {
            var s = state.stations[keys[i]];
            var ss = getStationState(keys[i]);
            var stateStr = ss.state || (s.Status === 'online' ? 'idle' : s.Status) || 'offline';
            var stateLabel = stateStr.charAt(0).toUpperCase() + stateStr.slice(1);
            var temps = state.stationTemps[keys[i]] || {};
            var t1 = formatTemp(temps.first_stage);
            var t2 = formatTemp(temps.second_stage);

            html += '<div class="station-card state-' + escapeHtml(stateStr) + '" onclick="App.openStation(\'' + escapeHtml(keys[i]) + '\')">';
            html += '<div class="station-card-header">';
            html += '<span class="station-name">' + escapeHtml(keys[i]) + '</span>';
            html += '<span class="status-badge ' + escapeHtml(stateStr) + '">' + stateLabel + '</span>';
            html += '</div>';

            if (stateStr !== 'offline') {
            html += '<div class="station-temps">';
            html += '<div class="temp-display"><span class="temp-label">1st Stage</span><span class="temp-value">' + t1 + '</span></div>';
            html += '<div class="temp-display"><span class="temp-label">2nd Stage</span><span class="temp-value">' + t2 + '</span></div>';
            html += '</div>';

            var ps = state.stationPumpStatus[keys[i]];
            if (ps) {
                var canControl = stateStr !== 'testing' && stateStr !== 'paused';
                var pDevId = escapeHtml(ps.device_id || '');
                var pInst = escapeHtml(keys[i]);
                html += '<div class="station-pump-status">';
                html += '<div class="pump-controls">';
                if (canControl) {
                    html += '<button class="pump-indicator ' + (ps.pump_on ? 'on' : 'off') + '" onclick="event.stopPropagation(); App.togglePump(\'' + pInst + '\', \'' + pDevId + '\')">' + (ps.pump_on ? 'PUMP ON' : 'PUMP OFF') + '</button>';
                    html += '<button class="pump-indicator ' + (ps.rough_valve_open ? 'off' : 'on') + '" onclick="event.stopPropagation(); App.toggleRoughValve(\'' + pInst + '\', \'' + pDevId + '\')">' + (ps.rough_valve_open ? 'ROUGH OPEN' : 'ROUGH CLOSED') + '</button>';
                    html += '<button class="pump-indicator ' + (ps.purge_valve_open ? 'off' : 'on') + '" onclick="event.stopPropagation(); App.togglePurgeValve(\'' + pInst + '\', \'' + pDevId + '\')">' + (ps.purge_valve_open ? 'PURGE OPEN' : 'PURGE CLOSED') + '</button>';
                } else {
                    html += '<span class="pump-indicator ' + (ps.pump_on ? 'on' : 'off') + '">' + (ps.pump_on ? 'PUMP ON' : 'PUMP OFF') + '</span>';
                    html += '<span class="pump-indicator ' + (ps.rough_valve_open ? 'off' : 'on') + '">' + (ps.rough_valve_open ? 'ROUGH OPEN' : 'ROUGH CLOSED') + '</span>';
                    html += '<span class="pump-indicator ' + (ps.purge_valve_open ? 'off' : 'on') + '">' + (ps.purge_valve_open ? 'PURGE OPEN' : 'PURGE CLOSED') + '</span>';
                }
                html += '</div>';
                if (ps.at_temp) html += '<span class="pump-flag at-temp">AT TEMP</span>';
                if (canControl) {
                    html += '<button class="pump-indicator ' + (ps.regen ? 'on' : 'regen-off') + '" onclick="event.stopPropagation(); App.toggleRegen(\'' + pInst + '\', \'' + pDevId + '\')">' + (ps.regen ? 'REGEN ON' : 'REGEN OFF') + '</button>';
                } else {
                    html += '<span class="pump-indicator ' + (ps.regen ? 'on' : 'regen-off') + '">' + (ps.regen ? 'REGEN ON' : 'REGEN OFF') + '</span>';
                }
                if (ps.regen && ps.regen_status) html += '<span class="regen-desc">' + escapeHtml(regenDescription(ps.regen_status)) + '</span>';
                html += '</div>';
            }

            var devices = s.Devices || [];
            html += '<div class="station-info-row"><span class="info-label">Devices:</span> ' + (devices.length > 0 ? devices.join(', ') : 'none') + '</div>';

            if (stateStr === 'testing' || stateStr === 'paused') {
                html += '<div class="station-info-row" style="color:var(--accent-blue)">Test in progress</div>';
            }
            }

            html += '</div>';
        }

        grid.innerHTML = html;
    }

    function openStation(instance) {
        state.detailStation = instance;
        state.tempChartData = { timestamps: [], first: [], second: [] };
        showView('station-detail');
    }

    // =================================================================
    // Station Detail
    // =================================================================
    function loadStationDetail(instance) {
        document.getElementById('detail-station-name').textContent = instance;

        // Load station state
        api('GET', '/stations/' + encodeURIComponent(instance) + '/state', null, function(err, data) {
            if (!err && data) {
                state.stationStates[instance] = data;
                renderStationDetail(instance);

                // Load temperature data if there's an active test
                if (data.test_run_id) {
                    loadTemperatureData(data.test_run_id);
                    loadTestEvents(data.test_run_id);
                }
            } else {
                renderStationDetail(instance);
            }
        });
    }

    function renderStationDetail(instance) {
        var s = state.stations[instance] || {};
        var ss = state.stationStates[instance] || {};
        var stateStr = ss.state || (s.Status === 'online' ? 'idle' : s.Status) || 'offline';
        var stateLabel = stateStr.charAt(0).toUpperCase() + stateStr.slice(1);

        // Status badge
        var badge = document.getElementById('detail-station-status');
        badge.textContent = stateLabel;
        badge.className = 'status-badge ' + stateStr;

        // Temps
        var temps = state.stationTemps[instance] || {};
        document.getElementById('detail-temp-1st').textContent = formatTemp(temps.first_stage) + ' K';
        document.getElementById('detail-temp-2nd').textContent = formatTemp(temps.second_stage) + ' K';

        // Elapsed
        document.getElementById('detail-elapsed').textContent = ss.started_at ? elapsed(ss.started_at) : '--';

        // Pump status
        var ps = state.stationPumpStatus[instance];
        var pumpHtml = '';
        if (ps) {
            var canControl = stateStr !== 'testing' && stateStr !== 'paused';
            var pDevId = escapeHtml(ps.device_id || '');
            var pInst = escapeHtml(instance);
            pumpHtml += '<div class="station-pump-status">';
            if (canControl) {
                pumpHtml += '<button class="pump-indicator ' + (ps.pump_on ? 'on' : 'off') + '" onclick="App.togglePump(\'' + pInst + '\', \'' + pDevId + '\')">' + (ps.pump_on ? 'PUMP ON' : 'PUMP OFF') + '</button>';
                pumpHtml += '<button class="pump-indicator ' + (ps.rough_valve_open ? 'off' : 'on') + '" onclick="App.toggleRoughValve(\'' + pInst + '\', \'' + pDevId + '\')">' + (ps.rough_valve_open ? 'ROUGH OPEN' : 'ROUGH CLOSED') + '</button>';
                pumpHtml += '<button class="pump-indicator ' + (ps.purge_valve_open ? 'off' : 'on') + '" onclick="App.togglePurgeValve(\'' + pInst + '\', \'' + pDevId + '\')">' + (ps.purge_valve_open ? 'PURGE OPEN' : 'PURGE CLOSED') + '</button>';
            } else {
                pumpHtml += '<span class="pump-indicator ' + (ps.pump_on ? 'on' : 'off') + '">' + (ps.pump_on ? 'PUMP ON' : 'PUMP OFF') + '</span>';
                pumpHtml += '<span class="pump-indicator ' + (ps.rough_valve_open ? 'off' : 'on') + '">' + (ps.rough_valve_open ? 'ROUGH OPEN' : 'ROUGH CLOSED') + '</span>';
                pumpHtml += '<span class="pump-indicator ' + (ps.purge_valve_open ? 'off' : 'on') + '">' + (ps.purge_valve_open ? 'PURGE OPEN' : 'PURGE CLOSED') + '</span>';
            }
            if (ps.at_temp) pumpHtml += '<span class="pump-flag at-temp">AT TEMP</span>';
            if (ps.regen) pumpHtml += '<span class="pump-flag regen">REGEN</span>';
            pumpHtml += '</div>';
        }
        document.getElementById('detail-pump-status').innerHTML = pumpHtml;

        // Station info
        var infoHtml = '';
        infoHtml += '<div class="station-info-row"><span class="info-label">Firmware:</span> ' + escapeHtml(s.FirmwareVersion || '--') + '</div>';
        infoHtml += '<div class="station-info-row"><span class="info-label">Uptime:</span> ' + formatUptime(s.UptimeSeconds) + '</div>';
        infoHtml += '<div class="station-info-row"><span class="info-label">Free Heap:</span> ' + formatBytes(s.FreeHeap) + '</div>';
        infoHtml += '<div class="station-info-row"><span class="info-label">Devices:</span> ' + ((s.Devices || []).join(', ') || 'none') + '</div>';
        document.getElementById('detail-station-info').innerHTML = infoHtml;

        // Test info
        var testInfoHtml = '';
        if (ss.script_name) {
            testInfoHtml += '<div class="station-info-row"><span class="info-label">Script:</span> ' + escapeHtml(ss.script_name) + '</div>';
        }
        if (ss.rma_id) {
            testInfoHtml += '<div class="station-info-row"><span class="info-label">RMA:</span> ' + escapeHtml(ss.rma_id) + '</div>';
        }
        if (ss.started_at) {
            testInfoHtml += '<div class="station-info-row"><span class="info-label">Started:</span> ' + formatDateTime(ss.started_at) + '</div>';
        }
        document.getElementById('detail-test-info').innerHTML = testInfoHtml || '<div style="color:var(--text-muted)">No active test</div>';

        // Controls
        var controlsHtml = '';
        if (stateStr === 'idle' || stateStr === 'online') {
            controlsHtml += '<button class="btn btn-primary" onclick="App.startTest(\'' + escapeHtml(instance) + '\')">Start Test</button>';
        } else if (stateStr === 'testing') {
            controlsHtml += '<div class="btn-group">';
            controlsHtml += '<button class="btn btn-warning" onclick="App.pauseTest(\'' + escapeHtml(instance) + '\')">Pause</button>';
            controlsHtml += '<button class="btn" onclick="App.terminateTest(\'' + escapeHtml(instance) + '\')">Terminate</button>';
            controlsHtml += '<button class="btn btn-danger btn-sm" onclick="App.abortTest(\'' + escapeHtml(instance) + '\')">Abort</button>';
            controlsHtml += '</div>';
        } else if (stateStr === 'paused') {
            controlsHtml += '<div class="btn-group">';
            controlsHtml += '<button class="btn btn-success" onclick="App.resumeTest(\'' + escapeHtml(instance) + '\')">Resume</button>';
            controlsHtml += '<button class="btn" onclick="App.terminateTest(\'' + escapeHtml(instance) + '\')">Terminate</button>';
            controlsHtml += '<button class="btn btn-danger btn-sm" onclick="App.abortTest(\'' + escapeHtml(instance) + '\')">Abort</button>';
            controlsHtml += '</div>';
        }
        document.getElementById('detail-controls').innerHTML = controlsHtml;

        // Manual command panel
        var manualPanel = document.getElementById('manual-cmd-panel');
        manualPanel.style.display = (stateStr === 'idle' || stateStr === 'online') ? 'block' : 'none';

        // Pre-fill device ID if we know it
        if (s.Devices && s.Devices.length > 0) {
            var devInput = document.getElementById('manual-device');
            if (!devInput.value) devInput.value = s.Devices[0];
        }
    }

    function loadTemperatureData(testRunId) {
        api('GET', '/test-runs/' + encodeURIComponent(testRunId) + '/temperatures', null, function(err, data) {
            if (!err && Array.isArray(data)) {
                state.tempChartData = { timestamps: [], first: [], second: [] };
                for (var i = 0; i < data.length; i++) {
                    var t = data[i];
                    state.tempChartData.timestamps.push(new Date(t.Timestamp).getTime());
                    if (t.Stage === 'first_stage') {
                        state.tempChartData.first.push(t.TemperatureK);
                        state.tempChartData.second.push(null);
                    } else {
                        state.tempChartData.first.push(null);
                        state.tempChartData.second.push(t.TemperatureK);
                    }
                }
                renderTempChart();
            }
        });
    }

    function loadTestEvents(testRunId) {
        api('GET', '/test-runs/' + encodeURIComponent(testRunId) + '/events', null, function(err, data) {
            if (!err && Array.isArray(data)) {
                renderTestEvents(data);
            }
        });
    }

    function renderTestEvents(events) {
        var tbody = document.getElementById('detail-events-tbody');
        if (!events || events.length === 0) {
            tbody.innerHTML = '<tr><td colspan="4"><div class="empty-state">No events</div></td></tr>';
            return;
        }
        var html = '';
        for (var i = 0; i < events.length; i++) {
            var e = events[i];
            html += '<tr>';
            html += '<td class="timestamp">' + formatDateTime(e.Timestamp) + '</td>';
            html += '<td class="mono">' + escapeHtml(e.EventType) + '</td>';
            html += '<td>' + escapeHtml(e.EmployeeID || '--') + '</td>';
            html += '<td>' + escapeHtml(e.Reason || '--') + '</td>';
            html += '</tr>';
        }
        tbody.innerHTML = html;
    }

    // =================================================================
    // Temperature Chart (Canvas)
    // =================================================================
    function renderTempChart() {
        var canvas = document.getElementById('temp-chart');
        if (!canvas) return;
        var ctx = canvas.getContext('2d');
        var rect = canvas.parentElement.getBoundingClientRect();

        canvas.width = rect.width * (window.devicePixelRatio || 1);
        canvas.height = rect.height * (window.devicePixelRatio || 1);
        canvas.style.width = rect.width + 'px';
        canvas.style.height = rect.height + 'px';
        ctx.scale(window.devicePixelRatio || 1, window.devicePixelRatio || 1);

        var w = rect.width;
        var h = rect.height;
        var pad = { top: 20, right: 20, bottom: 30, left: 55 };
        var plotW = w - pad.left - pad.right;
        var plotH = h - pad.top - pad.bottom;

        // Clear
        ctx.fillStyle = '#0f1629';
        ctx.fillRect(0, 0, w, h);

        var cd = state.tempChartData;
        // Build separate arrays for first/second stage
        var first = [];
        var second = [];
        var times = [];

        // Merge interleaved data into aligned arrays
        for (var i = 0; i < cd.timestamps.length; i++) {
            if (cd.first[i] != null) {
                first.push({ t: cd.timestamps[i], v: cd.first[i] });
            }
            if (cd.second[i] != null) {
                second.push({ t: cd.timestamps[i], v: cd.second[i] });
            }
        }

        if (first.length === 0 && second.length === 0) {
            ctx.fillStyle = '#5a6578';
            ctx.font = '14px sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText('No temperature data', w / 2, h / 2);
            return;
        }

        // Find data range
        var allTimes = first.map(function(p){return p.t}).concat(second.map(function(p){return p.t}));
        var tMin = Math.min.apply(null, allTimes);
        var tMax = Math.max.apply(null, allTimes);
        if (tMax === tMin) tMax = tMin + 60000;

        var allVals = first.map(function(p){return p.v}).concat(second.map(function(p){return p.v}));
        var vMin = Math.min.apply(null, allVals);
        var vMax = Math.max.apply(null, allVals);
        var vRange = vMax - vMin;
        if (vRange < 10) { vMin -= 5; vMax += 5; vRange = vMax - vMin; }
        vMin -= vRange * 0.05;
        vMax += vRange * 0.05;
        vRange = vMax - vMin;

        function xPos(t) { return pad.left + (t - tMin) / (tMax - tMin) * plotW; }
        function yPos(v) { return pad.top + (1 - (v - vMin) / vRange) * plotH; }

        // Grid lines
        ctx.strokeStyle = '#1e2a47';
        ctx.lineWidth = 1;
        var nGridY = 5;
        for (var g = 0; g <= nGridY; g++) {
            var gy = pad.top + g * plotH / nGridY;
            ctx.beginPath();
            ctx.moveTo(pad.left, gy);
            ctx.lineTo(w - pad.right, gy);
            ctx.stroke();

            var gv = vMax - g * vRange / nGridY;
            ctx.fillStyle = '#5a6578';
            ctx.font = '11px monospace';
            ctx.textAlign = 'right';
            ctx.fillText(gv.toFixed(0) + ' K', pad.left - 6, gy + 4);
        }

        // Time axis labels
        ctx.textAlign = 'center';
        ctx.fillStyle = '#5a6578';
        var nGridX = Math.min(6, Math.max(2, Math.floor(plotW / 80)));
        for (var gx = 0; gx <= nGridX; gx++) {
            var gt = tMin + gx * (tMax - tMin) / nGridX;
            var gxp = xPos(gt);
            var d = new Date(gt);
            ctx.fillText(d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }), gxp, h - 6);
        }

        // Plot first stage (blue)
        if (first.length > 1) {
            ctx.strokeStyle = '#4a9eff';
            ctx.lineWidth = 2;
            ctx.beginPath();
            ctx.moveTo(xPos(first[0].t), yPos(first[0].v));
            for (var f = 1; f < first.length; f++) {
                ctx.lineTo(xPos(first[f].t), yPos(first[f].v));
            }
            ctx.stroke();
        }

        // Plot second stage (green)
        if (second.length > 1) {
            ctx.strokeStyle = '#22c55e';
            ctx.lineWidth = 2;
            ctx.beginPath();
            ctx.moveTo(xPos(second[0].t), yPos(second[0].v));
            for (var s2 = 1; s2 < second.length; s2++) {
                ctx.lineTo(xPos(second[s2].t), yPos(second[s2].v));
            }
            ctx.stroke();
        }

        // Plot border
        ctx.strokeStyle = '#2a3a5c';
        ctx.lineWidth = 1;
        ctx.strokeRect(pad.left, pad.top, plotW, plotH);
    }

    // =================================================================
    // Test Control Actions
    // =================================================================
    function startTest(instance) {
        state.startTestStation = instance;
        state.startTestRMAId = null;
        document.getElementById('test-rma-search').value = '';
        document.getElementById('test-rma-results').innerHTML = '';
        document.getElementById('test-script').value = '';
        showError('start-test-error', '');
        openModal('start-test-modal');

        // Setup RMA search
        var searchInput = document.getElementById('test-rma-search');
        searchInput.oninput = function() {
            var q = searchInput.value.trim();
            if (q.length < 2) {
                document.getElementById('test-rma-results').innerHTML = '';
                state.startTestRMAId = null;
                return;
            }
            api('GET', '/rmas/search?q=' + encodeURIComponent(q), null, function(err, data) {
                if (err || !Array.isArray(data)) return;
                var html = '';
                for (var i = 0; i < data.length && i < 5; i++) {
                    html += '<div style="padding:4px 0;cursor:pointer;color:var(--accent-blue)" onclick="App.selectTestRMA(\'' + escapeHtml(data[i].ID) + '\',\'' + escapeHtml(data[i].RMANumber) + '\')">';
                    html += escapeHtml(data[i].RMANumber) + ' - ' + escapeHtml(data[i].CustomerName) + ' (' + escapeHtml(data[i].PumpSerialNumber) + ')';
                    html += '</div>';
                }
                if (data.length === 0) html = '<div style="color:var(--text-muted)">No matching RMAs</div>';
                document.getElementById('test-rma-results').innerHTML = html;
            });
        };
    }

    function selectTestRMA(id, number) {
        state.startTestRMAId = id;
        document.getElementById('test-rma-search').value = number;
        document.getElementById('test-rma-results').innerHTML = '<div style="color:var(--success-green)">Selected: ' + escapeHtml(number) + '</div>';
    }

    function confirmStartTest() {
        var instance = state.startTestStation;
        var rmaId = state.startTestRMAId;
        var script = document.getElementById('test-script').value.trim();

        if (!rmaId) { showError('start-test-error', 'Select an RMA'); return; }
        if (!script) { showError('start-test-error', 'Script path required'); return; }

        api('POST', '/stations/' + encodeURIComponent(instance) + '/test/start', {
            rma_id: rmaId,
            script_path: script
        }, function(err) {
            if (err) { showError('start-test-error', err.message); return; }
            closeModal('start-test-modal');
            loadStationDetail(instance);
        });
    }

    function pauseTest(instance) {
        api('POST', '/stations/' + encodeURIComponent(instance) + '/test/pause', null, function(err) {
            if (err) alert('Pause failed: ' + err.message);
            else loadStationDetail(instance);
        });
    }

    function resumeTest(instance) {
        api('POST', '/stations/' + encodeURIComponent(instance) + '/test/resume', null, function(err) {
            if (err) alert('Resume failed: ' + err.message);
            else loadStationDetail(instance);
        });
    }

    function terminateTest(instance) {
        state.startTestStation = instance;
        document.getElementById('terminate-reason').value = '';
        openModal('terminate-modal');
    }

    function confirmTerminate() {
        var instance = state.startTestStation;
        var reason = document.getElementById('terminate-reason').value.trim();
        api('POST', '/stations/' + encodeURIComponent(instance) + '/test/terminate', {
            reason: reason || 'operator terminated'
        }, function(err) {
            if (err) alert('Terminate failed: ' + err.message);
            closeModal('terminate-modal');
            loadStationDetail(instance);
        });
    }

    function abortTest(instance) {
        if (!confirm('Abort test? This will discard all collected data.')) return;
        api('POST', '/stations/' + encodeURIComponent(instance) + '/test/abort', null, function(err) {
            if (err) alert('Abort failed: ' + err.message);
            else loadStationDetail(instance);
        });
    }

    function togglePump(instance, deviceId) {
        var ps = state.stationPumpStatus[instance];
        var cmd = ps && ps.pump_on ? 'pump_off' : 'pump_on';
        if (ps) { ps.pump_on = !ps.pump_on; handlePumpStatus(ps); }
        api('POST', '/stations/' + encodeURIComponent(instance) + '/command', {
            device_id: deviceId, command: cmd
        }, function(){});
    }

    function toggleRoughValve(instance, deviceId) {
        var ps = state.stationPumpStatus[instance];
        var cmd = ps && ps.rough_valve_open ? 'close_rough_valve' : 'open_rough_valve';
        if (ps) { ps.rough_valve_open = !ps.rough_valve_open; handlePumpStatus(ps); }
        api('POST', '/stations/' + encodeURIComponent(instance) + '/command', {
            device_id: deviceId, command: cmd
        }, function(){});
    }

    function togglePurgeValve(instance, deviceId) {
        var ps = state.stationPumpStatus[instance];
        var cmd = ps && ps.purge_valve_open ? 'close_purge_valve' : 'open_purge_valve';
        if (ps) { ps.purge_valve_open = !ps.purge_valve_open; handlePumpStatus(ps); }
        api('POST', '/stations/' + encodeURIComponent(instance) + '/command', {
            device_id: deviceId, command: cmd
        }, function(){});
    }

    function toggleRegen(instance, deviceId) {
        var ps = state.stationPumpStatus[instance];
        var cmd = ps && ps.regen ? 'N0' : 'N1';
        if (ps) { ps.regen = !ps.regen; handlePumpStatus(ps); }
        api('POST', '/stations/' + encodeURIComponent(instance) + '/command', {
            device_id: deviceId, command: cmd
        }, function(){});
    }

    function sendManualCommand() {
        var instance = state.detailStation;
        var deviceId = document.getElementById('manual-device').value.trim();
        var command = document.getElementById('manual-command').value.trim();
        if (!deviceId || !command) return;

        document.getElementById('manual-response').textContent = 'Sending...';
        api('POST', '/stations/' + encodeURIComponent(instance) + '/command', {
            device_id: deviceId,
            command: command
        }, function(err, data) {
            if (err) {
                document.getElementById('manual-response').innerHTML = '<span style="color:var(--fail-red)">Error: ' + escapeHtml(err.message) + '</span>';
            } else if (data) {
                var respText = data.success ? data.response || 'OK' : 'FAIL: ' + (data.error || 'unknown');
                var color = data.success ? 'var(--success-green)' : 'var(--fail-red)';
                document.getElementById('manual-response').innerHTML = '<span style="color:' + color + '">' + escapeHtml(respText) + '</span>';
                if (data.duration_ms != null) {
                    document.getElementById('manual-response').innerHTML += ' <span class="duration-badge">' + data.duration_ms + ' ms</span>';
                }
            }
        });
    }

    // =================================================================
    // RMA Management
    // =================================================================
    function loadRMAs() {
        var statusFilter = document.getElementById('rma-status-filter').value;
        var url = '/rmas';
        if (statusFilter) url += '?status=' + statusFilter;

        api('GET', url, null, function(err, data) {
            if (err || !Array.isArray(data)) {
                document.getElementById('rma-list').innerHTML = '<div class="empty-state">Failed to load RMAs</div>';
                return;
            }
            renderRMAList(data);
        });
    }

    function searchRMAs() {
        var q = document.getElementById('rma-search-input').value.trim();
        if (q.length < 2) { loadRMAs(); return; }

        api('GET', '/rmas/search?q=' + encodeURIComponent(q), null, function(err, data) {
            if (err || !Array.isArray(data)) return;
            renderRMAList(data);
        });
    }

    function renderRMAList(rmas) {
        var listEl = document.getElementById('rma-list');
        if (rmas.length === 0) {
            listEl.innerHTML = '<div class="empty-state">No RMAs found</div>';
            return;
        }
        var html = '';
        for (var i = 0; i < rmas.length; i++) {
            var r = rmas[i];
            html += '<div class="rma-item ' + (r.Status === 'closed' ? 'closed' : '') + '" onclick="App.openRMA(\'' + escapeHtml(r.ID) + '\')">';
            html += '<div class="rma-info">';
            html += '<span class="rma-number">' + escapeHtml(r.RMANumber) + '</span>';
            html += '<div class="rma-meta">';
            html += '<span>' + escapeHtml(r.CustomerName) + '</span>';
            html += '<span>' + escapeHtml(r.PumpSerialNumber) + '</span>';
            html += '<span>' + escapeHtml(r.PumpModel) + '</span>';
            html += '</div>';
            html += '</div>';
            html += '<span class="rma-status-badge ' + (r.Status || 'open') + '">' + (r.Status || 'open') + '</span>';
            html += '</div>';
        }
        listEl.innerHTML = html;
    }

    function openRMA(id) {
        state.detailRMA = id;
        showView('rma-detail');
    }

    function loadRMADetail(id) {
        api('GET', '/rmas/' + encodeURIComponent(id), null, function(err, data) {
            if (err) {
                document.getElementById('rma-detail-info').innerHTML = '<div class="empty-state">Failed to load RMA</div>';
                return;
            }
            renderRMADetail(data);
        });
    }

    function renderRMADetail(rma) {
        if (!rma) return;
        document.getElementById('rma-detail-number').textContent = rma.RMANumber || rma.rma_number || '--';

        var status = rma.Status || rma.status || 'open';
        var statusBadge = document.getElementById('rma-detail-status');
        statusBadge.textContent = status;
        statusBadge.className = 'rma-status-badge ' + status;

        // Info blocks
        var infoHtml = '';
        infoHtml += '<div class="info-block"><div class="info-block-label">Customer</div><div class="info-block-value">' + escapeHtml(rma.CustomerName || rma.customer_name || '--') + '</div></div>';
        infoHtml += '<div class="info-block"><div class="info-block-label">Serial Number</div><div class="info-block-value">' + escapeHtml(rma.PumpSerialNumber || rma.pump_serial_number || '--') + '</div></div>';
        infoHtml += '<div class="info-block"><div class="info-block-label">Pump Model</div><div class="info-block-value">' + escapeHtml(rma.PumpModel || rma.pump_model || '--') + '</div></div>';
        document.getElementById('rma-detail-info').innerHTML = infoHtml;

        // Actions
        var actionsHtml = '';
        if (status === 'open') {
            actionsHtml += '<button class="btn btn-sm" onclick="App.downloadArtifact(\'' + escapeHtml(rma.ID || rma.id) + '\')">Download JSON</button>';
            actionsHtml += '<button class="btn btn-sm" onclick="App.downloadPDF(\'' + escapeHtml(rma.ID || rma.id) + '\')">Download PDF</button>';
            actionsHtml += '<button class="btn btn-sm btn-danger" onclick="App.closeRMA(\'' + escapeHtml(rma.ID || rma.id) + '\')">Close RMA</button>';
        } else {
            actionsHtml += '<button class="btn btn-sm" onclick="App.downloadArtifact(\'' + escapeHtml(rma.ID || rma.id) + '\')">Download JSON</button>';
            actionsHtml += '<button class="btn btn-sm" onclick="App.downloadPDF(\'' + escapeHtml(rma.ID || rma.id) + '\')">Download PDF</button>';
        }
        document.getElementById('rma-detail-actions').innerHTML = actionsHtml;

        // Test runs
        var rmaId = rma.ID || rma.id;
        api('GET', '/rmas/' + encodeURIComponent(rmaId), null, function(err, fullData) {
            // Fetch test runs for this RMA
            var runsEl = document.getElementById('rma-detail-runs');
            if (fullData && fullData.TestRuns && fullData.TestRuns.length > 0) {
                var html = '';
                for (var i = 0; i < fullData.TestRuns.length; i++) {
                    var run = fullData.TestRuns[i];
                    var statusClass = run.Status || 'error';
                    html += '<div class="test-run-item ' + escapeHtml(statusClass) + '">';
                    html += '<div class="test-run-info">';
                    html += '<div class="test-run-script">' + escapeHtml(run.ScriptName) + '</div>';
                    html += '<div class="test-run-meta">';
                    html += '<span>' + formatDateTime(run.StartedAt) + '</span>';
                    if (run.Summary) html += '<span>' + escapeHtml(run.Summary) + '</span>';
                    html += '</div></div>';
                    html += '<span class="test-run-status ' + escapeHtml(statusClass) + '">' + (statusClass.charAt(0).toUpperCase() + statusClass.slice(1)) + '</span>';
                    html += '</div>';
                }
                runsEl.innerHTML = html;
            } else {
                runsEl.innerHTML = '<div class="empty-state">No test runs for this RMA</div>';
            }
        });
    }

    function createRMA() {
        var rmaNumber = document.getElementById('rma-rma-number').value.trim();
        var serial = document.getElementById('rma-serial').value.trim();
        var customer = document.getElementById('rma-customer').value.trim();
        var model = document.getElementById('rma-model').value.trim();
        var notes = document.getElementById('rma-notes').value.trim();

        if (!rmaNumber || !serial || !customer || !model) {
            showError('rma-create-error', 'RMA number, serial, customer, and model are required');
            return;
        }

        api('POST', '/rmas', {
            rma_number: rmaNumber,
            pump_serial_number: serial,
            customer_name: customer,
            pump_model: model,
            notes: notes
        }, function(err, data) {
            if (err) {
                showError('rma-create-error', err.message);
                return;
            }
            // Clear form
            document.getElementById('rma-rma-number').value = '';
            document.getElementById('rma-serial').value = '';
            document.getElementById('rma-customer').value = '';
            document.getElementById('rma-model').value = '';
            document.getElementById('rma-notes').value = '';
            showError('rma-create-error', '');

            if (data && (data.id || data.ID)) {
                state.detailRMA = data.id || data.ID;
                showView('rma-detail');
            } else {
                showView('rma-list');
            }
        });
    }

    function closeRMA(id) {
        if (!confirm('Close this RMA? This will generate final artifacts and export to the file server.')) return;
        api('POST', '/rmas/' + encodeURIComponent(id) + '/close', null, function(err) {
            if (err) {
                alert('Failed to close RMA: ' + err.message);
                return;
            }
            loadRMADetail(id);
        });
    }

    function downloadArtifact(id) {
        window.open('/rmas/' + encodeURIComponent(id) + '/artifact', '_blank');
    }

    function downloadPDF(id) {
        window.open('/rmas/' + encodeURIComponent(id) + '/pdf', '_blank');
    }


    // =================================================================
    // Modals
    // =================================================================
    function openModal(id) {
        document.getElementById(id).classList.add('active');
    }
    function closeModal(id) {
        document.getElementById(id).classList.remove('active');
    }

    // =================================================================
    // WebSocket
    // =================================================================
    var ws = null;
    var wsReconnectDelay = 1000;

    function connectWebSocket() {
        if (ws && (ws.readyState === WebSocket.CONNECTING || ws.readyState === WebSocket.OPEN)) return;

        var protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        var url = protocol + '//' + window.location.host + '/ws';

        try { ws = new WebSocket(url); } catch(e) { scheduleReconnect(); return; }

        ws.onopen = function() {
            state.wsConnected = true;
            wsReconnectDelay = 1000;
            renderWSStatus();
            loadStations();
        };

        ws.onclose = function() {
            state.wsConnected = false;
            renderWSStatus();
            scheduleReconnect();
        };

        ws.onerror = function() {};

        ws.onmessage = function(event) {
            try {
                var msg = JSON.parse(event.data);
                handleWSMessage(msg);
            } catch(e) {}
        };
    }

    function scheduleReconnect() {
        setTimeout(function() { connectWebSocket(); }, wsReconnectDelay);
        wsReconnectDelay = Math.min(wsReconnectDelay * 1.5 + Math.random() * 500, 30000);
    }

    function renderWSStatus() {
        var el = document.getElementById('ws-indicator');
        if (state.wsConnected) {
            el.classList.add('connected');
        } else {
            el.classList.remove('connected');
        }
    }

    function handleWSMessage(msg) {
        if (!msg || !msg.type) return;

        switch (msg.type) {
            case 'heartbeat':
                handleHeartbeat(msg.payload);
                break;
            case 'estop':
                handleEstop(msg.payload);
                break;
            case 'station_state':
                handleStationState(msg.payload);
                break;
            case 'temperature':
                handleTemperature(msg.payload);
                break;
            case 'pump_status':
                handlePumpStatus(msg.payload);
                break;
            case 'test_event':
                handleTestEvent(msg.payload);
                break;
            case 'redis_health':
                // Could show a banner but not critical for operator
                break;
        }
    }

    function handleHeartbeat(payload) {
        if (!payload) return;

        // Match station by device list
        var keys = Object.keys(state.stations);
        for (var i = 0; i < keys.length; i++) {
            var s = state.stations[keys[i]];
            if (s.Devices && payload.devices && arraysEqual(s.Devices, payload.devices)) {
                s.Status = payload.status || 'online';
                s.UptimeSeconds = payload.uptime_seconds;
                s.FreeHeap = payload.free_heap;
                s.WifiRSSI = payload.wifi_rssi;
                s.FirmwareVersion = payload.firmware_version;
                break;
            }
        }

        if (state.currentView === 'stations') renderStationGrid();
    }

    function arraysEqual(a, b) {
        if (!a && !b) return true;
        if (!a || !b || a.length !== b.length) return false;
        var sa = a.slice().sort(), sb = b.slice().sort();
        for (var i = 0; i < sa.length; i++) if (sa[i] !== sb[i]) return false;
        return true;
    }


    function handleEstop(payload) {
        if (!payload) return;
        state.estop = {
            active: !!payload.active,
            reason: payload.reason || '',
            description: payload.description || '',
            initiator: payload.initiator || '',
            triggered_at: payload.triggered_at || new Date().toISOString()
        };
        renderEstop();
    }

    function handleStationState(payload) {
        if (!payload || !payload.instance) return;
        state.stationStates[payload.instance] = payload;

        if (state.currentView === 'stations') renderStationGrid();
        if (state.currentView === 'station-detail' && state.detailStation === payload.instance) {
            renderStationDetail(payload.instance);
        }
    }

    function handleTemperature(payload) {
        if (!payload) return;

        // Update live temp display
        var instance = payload.station_instance;
        if (instance) {
            if (!state.stationTemps[instance]) state.stationTemps[instance] = {};
            if (payload.stage === 'first_stage') {
                state.stationTemps[instance].first_stage = payload.temperature_k;
            } else if (payload.stage === 'second_stage') {
                state.stationTemps[instance].second_stage = payload.temperature_k;
            }
        }

        // Update chart if viewing this station
        if (state.currentView === 'station-detail' && state.detailStation === instance) {
            var ts = payload.timestamp ? new Date(payload.timestamp).getTime() : Date.now();
            state.tempChartData.timestamps.push(ts);
            state.tempChartData.first.push(payload.stage === 'first_stage' ? payload.temperature_k : null);
            state.tempChartData.second.push(payload.stage === 'second_stage' ? payload.temperature_k : null);

            // Trim to max points
            while (state.tempChartData.timestamps.length > MAX_CHART_POINTS) {
                state.tempChartData.timestamps.shift();
                state.tempChartData.first.shift();
                state.tempChartData.second.shift();
            }

            var temps = state.stationTemps[instance] || {};
            document.getElementById('detail-temp-1st').textContent = formatTemp(temps.first_stage) + ' K';
            document.getElementById('detail-temp-2nd').textContent = formatTemp(temps.second_stage) + ' K';

            renderTempChart();
        }

        if (state.currentView === 'stations') renderStationGrid();
    }

    function handlePumpStatus(payload) {
        if (!payload || !payload.station_instance) return;
        state.stationPumpStatus[payload.station_instance] = payload;
        if (state.currentView === 'stations') renderStationGrid();
        if (state.currentView === 'station-detail' && state.detailStation === payload.station_instance) {
            renderStationDetail(payload.station_instance);
        }
    }

    function handleTestEvent(payload) {
        if (!payload) return;
        // Refresh station detail if viewing
        if (state.currentView === 'station-detail' && payload.test_run_id) {
            loadTestEvents(payload.test_run_id);
        }
    }

    function renderEstop() {
        var es = state.estop;
        var banner = document.getElementById('estop-banner');
        var badge = document.getElementById('estop-status-badge');

        if (es.active) {
            banner.classList.add('active');
            document.getElementById('estop-reason').textContent = es.reason || 'Unknown reason';
            document.getElementById('estop-description').textContent = es.description || '';
            document.getElementById('estop-initiator').textContent = es.initiator || 'unknown';
            document.getElementById('estop-time').textContent = formatDateTime(es.triggered_at);
            badge.textContent = 'E-STOP';
            badge.className = 'estop-status-badge active';
        } else {
            banner.classList.remove('active');
            badge.textContent = 'SAFE';
            badge.className = 'estop-status-badge safe';
        }
    }

    // =================================================================
    // Periodic Updates
    // =================================================================
    setInterval(function() {
        if (state.currentView === 'stations' && state.employee) {
            loadStations();
        }
    }, 3000);

    setInterval(function() {
        if (state.currentView === 'station-detail' && state.detailStation) {
            var ss = state.stationStates[state.detailStation] || {};
            if (ss.started_at) {
                document.getElementById('detail-elapsed').textContent = elapsed(ss.started_at);
            }
        }
    }, 1000);

    // Auto-login with pre-filled credentials
    login();

    // Handle enter key on login
    document.addEventListener('keydown', function(e) {
        if (e.key === 'Enter') {
            if (state.currentView === 'login') login();
            if (document.getElementById('manual-command') === document.activeElement) sendManualCommand();
        }
    });

    // =================================================================
    // Public API
    // =================================================================
    return {
        showView: showView,
        login: login,
        logout: logout,
        openStation: openStation,
        startTest: startTest,
        selectTestRMA: selectTestRMA,
        confirmStartTest: confirmStartTest,
        pauseTest: pauseTest,
        resumeTest: resumeTest,
        terminateTest: terminateTest,
        confirmTerminate: confirmTerminate,
        abortTest: abortTest,
        togglePump: togglePump,
        toggleRoughValve: toggleRoughValve,
        togglePurgeValve: togglePurgeValve,
        toggleRegen: toggleRegen,
        sendManualCommand: sendManualCommand,
        loadRMAs: loadRMAs,
        searchRMAs: searchRMAs,
        openRMA: openRMA,
        createRMA: createRMA,
        closeRMA: closeRMA,
        closeModal: closeModal,
        downloadArtifact: downloadArtifact,
        downloadPDF: downloadPDF
    };
})();
