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
        tempChartData: { timestamps: [], first: [], second: [] },
        tempWindowHours: null,  // null = autorange, number = hours preset
        userZoom: null          // {x0, x1, y0, y1} when user drags a zoom region
    };

    var MAX_CHART_POINTS = 17280; // 12 hours at 5s intervals, 2 stages interleaved

    // =================================================================
    // Theme
    // =================================================================
    var THEMES = ['industrial', 'contrast', 'light'];
    var THEME_LABELS = { industrial: 'Industrial', contrast: 'Hi-Con', light: 'Light' };

    function initTheme() {
        var saved = localStorage.getItem('theme') || 'industrial';
        if (THEMES.indexOf(saved) === -1) saved = 'industrial';
        applyTheme(saved);
    }

    function applyTheme(theme) {
        if (theme === 'industrial') {
            delete document.documentElement.dataset.theme;
        } else {
            document.documentElement.dataset.theme = theme;
        }
        var label = document.getElementById('theme-label');
        if (label) label.textContent = THEME_LABELS[theme] || theme;
    }

    function toggleTheme() {
        var current = document.documentElement.dataset.theme || 'dark';
        var idx = THEMES.indexOf(current);
        var next = THEMES[(idx + 1) % THEMES.length];
        applyTheme(next);
        localStorage.setItem('theme', next);
        // Defer so computed styles settle before Plotly reads them
        requestAnimationFrame(function() {
            var el = document.getElementById('temp-chart');
            if (el && el.data) {
                Plotly.relayout(el, {
                    paper_bgcolor: chartColors().paper,
                    plot_bgcolor: chartColors().paper,
                    'font.color': chartColors().text,
                    'xaxis.gridcolor': chartColors().grid,
                    'xaxis.linecolor': chartColors().line,
                    'yaxis.gridcolor': chartColors().grid,
                    'yaxis.linecolor': chartColors().line,
                    'legend.font.color': chartColors().legend,
                    'shapes[0].line.color': chartColors().line,
                    'shapes[1].line.color': '#22c55e'
                });
            }
        });
    }

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
                return 'Purge Gas Failure';
            case 'H': return 'Extended Purge';
            case 'S': return 'Repurge Cycle';
            case 'I': case 'J': case 'K': case 'T':
            case 'a': case 'b': case 'j': case 'n':
                return 'Rough to Base Pressure';
            case 'L': return 'Rate of Rise';
            case 'M': case 'N': case 'c': case 'd': case 'o':
                return 'Cooldown';
            case 'P': return 'Regen Complete';
            case 'U': return 'FastRegen Start';
            case 'V': return 'Regen Aborted';
            case 'W': return 'Delay Restart';
            case 'X': case 'Y': return 'Power Failure';
            case 'Z': return 'Delay Start';
            case 'O': case '[': return 'Zeroing TC Gauge';
            case 'f': return 'Share Regen Wait';
            case 'e': return 'Repurge (FastRegen)';
            case 'h': return 'Purge Coordinate Wait';
            case 'i': return 'Rough Coordinate Wait';
            case 'k': return 'Purge Gas Fail Recovery';
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
            return d.toLocaleTimeString('en-US', {
                timeZone: 'America/Denver',
                hour: '2-digit', minute: '2-digit', second: '2-digit'
            });
        } catch(e) { return '--'; }
    }

    function formatDateTime(iso) {
        if (!iso) return '--';
        try {
            var d = new Date(iso);
            if (isNaN(d.getTime())) return '--';
            return d.toLocaleString('en-US', {
                timeZone: 'America/Denver',
                month:'short', day:'numeric', hour:'2-digit', minute:'2-digit', second:'2-digit'
            });
        } catch(e) { return '--'; }
    }

    function scriptLabel(path) {
        if (!path) return '--';
        var name = path.split('/').pop();
        return name;
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

    function buildPIDSchematic(ps, canControl, pInst, pDevId, stopProp) {
        var sp = stopProp ? 'event.stopPropagation(); ' : '';
        var pumpTag = canControl ? 'button' : 'div';
        var pumpClick = canControl ? ' onclick="' + sp + 'App.togglePump(\'' + pInst + '\', \'' + pDevId + '\')"' : '';
        var h = '<div class="pid-schematic">';
        h += '<div class="pid-main">';
        // Pump motor
        h += '<' + pumpTag + ' class="pid-pump ' + (ps.pump_on ? 'on' : 'off') + '"' + pumpClick + '>';
        h += '<span class="pid-pump-symbol"></span>';
        h += '<span class="pid-pump-label">PUMP<br><small>' + (ps.pump_on ? 'Running' : 'Stopped') + '</small></span>';
        h += '</' + pumpTag + '>';
        // Trunk line
        h += '<span class="pid-trunk' + (ps.pump_on ? ' active' : '') + '"></span>';
        // Branch split
        h += '<div class="pid-branches' + (ps.pump_on ? ' active' : '') + '">';
        // Rough valve branch
        var roughTag = canControl ? 'button' : 'div';
        var roughClick = canControl ? ' onclick="' + sp + 'App.toggleRoughValve(\'' + pInst + '\', \'' + pDevId + '\')"' : '';
        h += '<div class="pid-branch' + (ps.rough_valve_open ? ' active' : '') + '">';
        h += '<span class="pid-line"></span>';
        h += '<' + roughTag + ' class="pid-valve ' + (ps.rough_valve_open ? 'open' : 'closed') + '"' + roughClick + '>';
        h += '<span class="pid-valve-symbol"><span class="pid-valve-tri-l"></span><span class="pid-valve-tri-r"></span></span>';
        h += '<span class="pid-valve-label">ROUGH <small>' + (ps.rough_valve_open ? 'Open' : 'Closed') + '</small></span>';
        h += '</' + roughTag + '>';
        h += '</div>';
        // Purge valve branch
        var purgeTag = canControl ? 'button' : 'div';
        var purgeClick = canControl ? ' onclick="' + sp + 'App.togglePurgeValve(\'' + pInst + '\', \'' + pDevId + '\')"' : '';
        h += '<div class="pid-branch' + (ps.purge_valve_open ? ' active' : '') + '">';
        h += '<span class="pid-line"></span>';
        h += '<' + purgeTag + ' class="pid-valve ' + (ps.purge_valve_open ? 'open' : 'closed') + '"' + purgeClick + '>';
        h += '<span class="pid-valve-symbol"><span class="pid-valve-tri-l"></span><span class="pid-valve-tri-r"></span></span>';
        h += '<span class="pid-valve-label">PURGE <small>' + (ps.purge_valve_open ? 'Open' : 'Closed') + '</small></span>';
        h += '</' + purgeTag + '>';
        h += '</div>';
        h += '</div>'; // pid-branches
        h += '</div>'; // pid-main
        // Regen + AT TEMP row
        h += '<div class="pid-regen-row">';
        var regenTag = canControl ? 'button' : 'div';
        var regenClick = canControl ? ' onclick="' + sp + 'App.toggleRegen(\'' + pInst + '\', \'' + pDevId + '\')"' : '';
        h += '<' + regenTag + ' class="pid-regen ' + (ps.regen ? 'on' : 'off') + '"' + regenClick + '>';
        h += '<span class="pid-regen-symbol"></span>';
        h += '<span>REGEN <small>' + (ps.regen ? 'On' : 'Off') + '</small></span>';
        h += '</' + regenTag + '>';
        if (ps.at_temp) h += '<span class="pump-flag at-temp">AT TEMP</span>';
        h += '</div>';
        if (ps.regen && ps.regen_status) h += '<span class="regen-desc">' + escapeHtml(regenDescription(ps.regen_status)) + '</span>';
        h += '</div>'; // pid-schematic
        return h;
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

            var stationNum = keys[i].replace(/\D+/g, '').replace(/^0+/, '') || keys[i];
            html += '<div class="station-card state-' + escapeHtml(stateStr) + '" onclick="App.openStation(\'' + escapeHtml(keys[i]) + '\')">';
            html += '<div class="station-top-row">';
            html += '<span class="station-name">' + escapeHtml(stationNum) + '</span>';
            html += '<span class="temp-label">1st</span><span class="temp-value">' + t1 + '</span>';
            html += '<span class="temp-label">2nd</span><span class="temp-value">' + t2 + '</span>';
            html += '<span class="status-badge ' + escapeHtml(stateStr) + '">' + stateLabel + '</span>';
            html += '</div>';

            if (stateStr !== 'offline') {

            var ps = state.stationPumpStatus[keys[i]];
            if (ps) {
                var canControl = stateStr !== 'testing' && stateStr !== 'paused';
                var pDevId = escapeHtml(ps.device_id || '');
                var pInst = escapeHtml(keys[i]);
                html += '<div class="station-pump-status">';
                html += buildPIDSchematic(ps, canControl, pInst, pDevId, true);
                html += '</div>';
            }

            var devices = s.Devices || [];
            var deviceTypes = s.DeviceTypes || {};
            var deviceLabels = devices.map(function(d) {
                var t = deviceTypes[d];
                return t ? d + ' (' + t + ')' : d;
            });
            html += '<div class="station-info-row"><span class="info-label">Devices:</span> ' + (deviceLabels.length > 0 ? deviceLabels.join(', ') : 'none') + '</div>';

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
        state.tempWindowHours = null;
        state.userZoom = null;
        showView('station-detail');
    }

    // =================================================================
    // Station Detail
    // =================================================================
    function loadContinuousTemperatures(instance) {
        api('GET', '/stations/' + encodeURIComponent(instance) + '/temperatures', null, function(err, data) {
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
                // Trim to max so first WebSocket point doesn't cause a sudden jump
                while (state.tempChartData.timestamps.length > MAX_CHART_POINTS) {
                    state.tempChartData.timestamps.shift();
                    state.tempChartData.first.shift();
                    state.tempChartData.second.shift();
                }
                renderTempChart();
            }
        });
    }

    function loadStationDetail(instance) {
        document.getElementById('detail-station-name').textContent = instance;

        // Always load continuous temperature history (12h rolling window)
        loadContinuousTemperatures(instance);

        // Load station state
        api('GET', '/stations/' + encodeURIComponent(instance) + '/state', null, function(err, data) {
            if (!err && data) {
                state.stationStates[instance] = data;
                renderStationDetail(instance);

                // Load test events from active or most recent test
                var eventRunId = data.test_run_id || data.last_test_run_id;
                if (eventRunId) {
                    loadTestEvents(eventRunId);
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
        document.getElementById('detail-temp-1st').textContent = formatTemp(temps.first_stage);
        document.getElementById('detail-temp-2nd').textContent = formatTemp(temps.second_stage);

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
            pumpHtml += buildPIDSchematic(ps, canControl, pInst, pDevId, false);
            pumpHtml += '</div>';
        }
        document.getElementById('detail-pump-status').innerHTML = pumpHtml;

        // Station info
        var infoHtml = '';
        infoHtml += '<div class="station-info-row"><span class="info-label">Firmware:</span> ' + escapeHtml(s.FirmwareVersion || '--') + '</div>';
        infoHtml += '<div class="station-info-row"><span class="info-label">Uptime:</span> ' + formatUptime(s.UptimeSeconds) + '</div>';
        infoHtml += '<div class="station-info-row"><span class="info-label">Free Heap:</span> ' + formatBytes(s.FreeHeap) + '</div>';
        var detDevices = s.Devices || [];
        var detDeviceTypes = s.DeviceTypes || {};
        var detDeviceLabels = detDevices.map(function(d) {
            var t = detDeviceTypes[d];
            return t ? d + ' (' + t + ')' : d;
        });
        infoHtml += '<div class="station-info-row"><span class="info-label">Devices:</span> ' + (detDeviceLabels.join(', ') || 'none') + '</div>';
        document.getElementById('detail-station-info').innerHTML = infoHtml;

        // Test info
        var testInfoHtml = '';
        if (ss.script_name) {
            testInfoHtml += '<div class="station-info-row"><span class="info-label">Script:</span> ' + escapeHtml(scriptLabel(ss.script_name)) + '</div>';
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
            controlsHtml += '<button class="btn" title="Stop test and save collected data" onclick="App.terminateTest(\'' + escapeHtml(instance) + '\')">Terminate</button>';
            controlsHtml += '<button class="btn btn-danger" title="Stop test and discard all data" style="margin-left:auto" onclick="App.abortTest(\'' + escapeHtml(instance) + '\')">Abort</button>';
            controlsHtml += '</div>';
        } else if (stateStr === 'paused') {
            controlsHtml += '<div class="btn-group">';
            controlsHtml += '<button class="btn btn-success" onclick="App.resumeTest(\'' + escapeHtml(instance) + '\')">Resume</button>';
            controlsHtml += '<button class="btn" title="Stop test and save collected data" onclick="App.terminateTest(\'' + escapeHtml(instance) + '\')">Terminate</button>';
            controlsHtml += '<button class="btn btn-danger" title="Stop test and discard all data" style="margin-left:auto" onclick="App.abortTest(\'' + escapeHtml(instance) + '\')">Abort</button>';
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
            html += '<td>' + escapeHtml(e.Reason || e.Description || '--') + '</td>';
            html += '</tr>';
        }
        tbody.innerHTML = html;
    }

    // =================================================================
    // Temperature Chart (Plotly)
    // =================================================================
    function chartColors() {
        var s = getComputedStyle(document.documentElement);
        return {
            paper: s.getPropertyValue('--bg-input').trim(),
            grid: s.getPropertyValue('--bg-card').trim(),
            line: s.getPropertyValue('--border-color').trim(),
            text: s.getPropertyValue('--text-muted').trim(),
            legend: s.getPropertyValue('--text-secondary').trim()
        };
    }

    function renderTempChart() {
        var el = document.getElementById('temp-chart');
        if (!el) return;

        var cd = state.tempChartData;
        var cc = chartColors();

        // Build separate arrays for first/second stage
        var firstX = [], firstY = [], secondX = [], secondY = [];
        for (var i = 0; i < cd.timestamps.length; i++) {
            if (cd.first[i] != null) {
                firstX.push(new Date(cd.timestamps[i]));
                firstY.push(cd.first[i]);
            }
            if (cd.second[i] != null) {
                secondX.push(new Date(cd.timestamps[i]));
                secondY.push(cd.second[i]);
            }
        }

        var traces = [
            {
                x: firstX, y: firstY,
                name: '1st Stage',
                type: 'scattergl',
                mode: 'lines',
                line: { color: '#4a9eff', width: 2 },
                connectgaps: false
            },
            {
                x: secondX, y: secondY,
                name: '2nd Stage',
                type: 'scattergl',
                mode: 'lines',
                line: { color: '#22c55e', width: 2 },
                connectgaps: false
            }
        ];

        var layout = {
            paper_bgcolor: cc.paper,
            plot_bgcolor: cc.paper,
            font: { color: cc.text, family: 'monospace' },
            margin: { t: 20, r: 20, b: 40, l: 55 },
            xaxis: {
                type: 'date',
                gridcolor: cc.grid,
                linecolor: cc.line,
                tickformat: '%I:%M %p',
                hoverformat: '%I:%M:%S %p',
                tickfont: { size: 24 },
                autorange: !state.userZoom && state.tempWindowHours == null,
                range: state.userZoom
                    ? [state.userZoom.x0, state.userZoom.x1]
                    : state.tempWindowHours != null
                        ? [new Date(Date.now() - state.tempWindowHours * 3600000), new Date()]
                        : undefined
            },
            yaxis: {
                range: state.userZoom ? [state.userZoom.y0, state.userZoom.y1] : [0, 320],
                autorange: false,
                tickmode: 'array',
                tickvals: [0, 20, 40, 80, 120, 160, 200, 240, 280, 320],
                title: '',
                tickfont: { size: 24 },
                gridcolor: cc.grid,
                linecolor: cc.line,
                showline: true
            },
            legend: {
                orientation: 'h',
                x: 0.5, xanchor: 'center',
                y: 1.02, yanchor: 'bottom',
                font: { color: cc.legend, size: 24 }
            },
            hovermode: 'x unified',
            shapes: [
                {
                    type: 'line',
                    x0: 0, x1: 1, xref: 'paper',
                    y0: 320, y1: 320, yref: 'y',
                    line: { color: cc.line, width: 1 }
                },
                {
                    type: 'line',
                    x0: 0, x1: 1, xref: 'paper',
                    y0: 20, y1: 20, yref: 'y',
                    line: { color: '#22c55e', width: 1, dash: 'dash' }
                }
            ]
        };

        var config = {
            responsive: true,
            displaylogo: false,
            modeBarButtonsToRemove: ['lasso2d', 'select2d']
        };

        Plotly.react(el, traces, layout, config);

        // Track user zoom / reset
        el.removeAllListeners('plotly_relayout');
        el.on('plotly_relayout', function(ev) {
            if (ev['xaxis.autorange'] || ev['yaxis.autorange']) {
                state.userZoom = null;
                state.tempWindowHours = null;
            } else if (ev['xaxis.range[0]'] && ev['xaxis.range[1]'] &&
                       ev['yaxis.range[0]'] != null && ev['yaxis.range[1]'] != null) {
                state.userZoom = {
                    x0: ev['xaxis.range[0]'],
                    x1: ev['xaxis.range[1]'],
                    y0: ev['yaxis.range[0]'],
                    y1: ev['yaxis.range[1]']
                };
                state.tempWindowHours = null;
            }
        });
    }

    function setTempWindow(hours) {
        state.tempWindowHours = hours;
        state.userZoom = null;
        renderTempChart();
    }

    function exportTempCSV() {
        var cd = state.tempChartData;
        if (cd.timestamps.length === 0) return;

        // Determine time range
        var x0, x1;
        if (state.userZoom) {
            x0 = new Date(state.userZoom.x0).getTime();
            x1 = new Date(state.userZoom.x1).getTime();
        }

        // Normalize timestamps to the second and merge first/second into one row
        var rows = {};
        for (var i = 0; i < cd.timestamps.length; i++) {
            var ts = cd.timestamps[i];
            if (x0 != null && (ts < x0 || ts > x1)) continue;
            var sec = Math.floor(ts / 1000) * 1000; // truncate to second
            if (!rows[sec]) rows[sec] = { first: null, second: null };
            if (cd.first[i] != null) rows[sec].first = cd.first[i];
            if (cd.second[i] != null) rows[sec].second = cd.second[i];
        }

        var keys = Object.keys(rows).sort(function(a, b) { return a - b; });
        if (keys.length === 0) return;

        // Format timestamp in America/Denver timezone
        var fmt = new Intl.DateTimeFormat('en-US', {
            timeZone: 'America/Denver',
            year: 'numeric', month: '2-digit', day: '2-digit',
            hour: '2-digit', minute: '2-digit', second: '2-digit',
            hour12: false
        });

        var csv = 'Timestamp (MST/MDT),1st Stage (K),2nd Stage (K)\n';
        for (var j = 0; j < keys.length; j++) {
            var r = rows[keys[j]];
            var d = new Date(Number(keys[j]));
            var parts = fmt.formatToParts(d);
            var p = {};
            for (var k = 0; k < parts.length; k++) p[parts[k].type] = parts[k].value;
            var tsStr = p.year + '-' + p.month + '-' + p.day + ' ' + p.hour + ':' + p.minute + ':' + p.second;
            csv += tsStr + ',' + (r.first != null ? r.first : '') + ',' + (r.second != null ? r.second : '') + '\n';
        }

        var station = state.detailStation || 'station';
        var date = new Date().toISOString().slice(0, 10);
        var filename = station + '-temps-' + date + '.csv';

        var blob = new Blob([csv], { type: 'text/csv' });
        var url = URL.createObjectURL(blob);
        var a = document.createElement('a');
        a.href = url;
        a.download = filename;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);
    }

    // =================================================================
    // Test Control Actions
    // =================================================================
    function startTest(instance) {
        state.startTestStation = instance;
        showError('start-test-error', '');

        // Show step 1, hide step 2
        document.getElementById('start-test-step-rma').style.display = '';
        document.getElementById('start-test-step-script').style.display = 'none';
        openModal('start-test-modal');

        // Populate RMA grid
        var grid = document.getElementById('start-test-rma-grid');
        grid.innerHTML = '<div class="empty-state">Loading...</div>';
        api('GET', '/rmas?status=open', null, function(err, data) {
            if (err || !Array.isArray(data) || data.length === 0) {
                grid.innerHTML = '<div class="empty-state">No open RMAs</div>';
                return;
            }
            var html = '';
            for (var i = 0; i < data.length; i++) {
                var r = data[i];
                html += '<button class="start-test-btn" onclick="App.selectRMAForTest(\'' + escapeHtml(r.ID) + '\')">';
                html += '<div class="start-test-btn-title">' + escapeHtml(r.RMANumber) + '</div>';
                html += '<div class="start-test-btn-detail">' + escapeHtml(r.CustomerName) + '</div>';
                html += '<div class="start-test-btn-detail">' + escapeHtml(r.PumpSerialNumber) + '</div>';
                html += '</button>';
            }
            grid.innerHTML = html;
        });
    }

    function selectRMAForTest(rmaId) {
        state.startTestRMA = rmaId;

        // Show step 2, hide step 1
        document.getElementById('start-test-step-rma').style.display = 'none';
        document.getElementById('start-test-step-script').style.display = '';

        // Populate script grid
        var grid = document.getElementById('start-test-script-grid');
        grid.innerHTML = '<div class="empty-state">Loading...</div>';
        api('GET', '/scripts', null, function(err, data) {
            if (err || !Array.isArray(data) || data.length === 0) {
                grid.innerHTML = '<div class="empty-state">No scripts found</div>';
                return;
            }
            var html = '';
            for (var i = 0; i < data.length; i++) {
                html += '<button class="start-test-btn" onclick="App.selectScriptForTest(\'' + escapeHtml(data[i].path) + '\')">';
                html += '<div class="start-test-btn-title">' + escapeHtml(scriptLabel(data[i].name)) + '</div>';
                html += '</button>';
            }
            grid.innerHTML = html;
        });
    }

    function selectScriptForTest(scriptPath) {
        var instance = state.startTestStation;
        var rmaId = state.startTestRMA;

        api('POST', '/stations/' + encodeURIComponent(instance) + '/test/start', {
            rma_id: rmaId,
            script_path: scriptPath
        }, function(err) {
            if (err) { showError('start-test-error', err.message); backToRMASelect(); return; }
            closeModal('start-test-modal');
            loadStationDetail(instance);
        });
    }

    function backToRMASelect() {
        document.getElementById('start-test-step-script').style.display = 'none';
        document.getElementById('start-test-step-rma').style.display = '';
    }

    function pauseTest(instance) {
        api('POST', '/stations/' + encodeURIComponent(instance) + '/test/pause', null, function(err) {
            if (err) showToast('Pause failed: ' + err.message, 'error');
            else loadStationDetail(instance);
        });
    }

    function resumeTest(instance) {
        api('POST', '/stations/' + encodeURIComponent(instance) + '/test/resume', null, function(err) {
            if (err) showToast('Resume failed: ' + err.message, 'error');
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
            if (err) showToast('Terminate failed: ' + err.message, 'error');
            closeModal('terminate-modal');
            loadStationDetail(instance);
        });
    }

    function abortTest(instance) {
        showConfirm(
            'Abort Test',
            'This will discard all collected data. Are you sure?',
            'Abort', 'btn-danger',
            function(confirmed) {
                if (!confirmed) return;
                api('POST', '/stations/' + encodeURIComponent(instance) + '/test/abort', null, function(err) {
                    if (err) showToast('Abort failed: ' + err.message, 'error');
                    else loadStationDetail(instance);
                });
            }
        );
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
        var cmd = ps && ps.regen ? 'abort_regen' : 'start_regen';
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
            // API returns {"rma": {...}, "runs": [...]}
            var rma = data && data.rma ? data.rma : data;
            var runs = data && data.runs ? data.runs : [];
            renderRMADetail(rma, runs);
        });
    }

    function renderRMADetail(rma, runs) {
        if (!rma) return;
        document.getElementById('rma-detail-number').textContent = rma.RMANumber || '--';

        var status = rma.Status || 'open';
        var statusBadge = document.getElementById('rma-detail-status');
        statusBadge.textContent = status;
        statusBadge.className = 'rma-status-badge ' + status;

        // Info blocks
        var infoHtml = '';
        infoHtml += '<div class="info-block"><div class="info-block-label">Customer</div><div class="info-block-value">' + escapeHtml(rma.CustomerName || '--') + '</div></div>';
        infoHtml += '<div class="info-block"><div class="info-block-label">Serial Number</div><div class="info-block-value">' + escapeHtml(rma.PumpSerialNumber || '--') + '</div></div>';
        infoHtml += '<div class="info-block"><div class="info-block-label">Pump Model</div><div class="info-block-value">' + escapeHtml(rma.PumpModel || '--') + '</div></div>';
        document.getElementById('rma-detail-info').innerHTML = infoHtml;

        // Actions
        var actionsHtml = '';
        if (status === 'open') {
            actionsHtml += '<button class="btn btn-sm" onclick="App.downloadArtifact(\'' + escapeHtml(rma.ID) + '\')">Download JSON</button>';
            actionsHtml += '<button class="btn btn-sm" onclick="App.downloadPDF(\'' + escapeHtml(rma.ID) + '\')">Download PDF</button>';
            actionsHtml += '<button class="btn btn-sm btn-danger" style="margin-left:24px" onclick="App.closeRMA(\'' + escapeHtml(rma.ID) + '\')">Close RMA</button>';
        } else {
            actionsHtml += '<button class="btn btn-sm" onclick="App.downloadArtifact(\'' + escapeHtml(rma.ID) + '\')">Download JSON</button>';
            actionsHtml += '<button class="btn btn-sm" onclick="App.downloadPDF(\'' + escapeHtml(rma.ID) + '\')">Download PDF</button>';
        }
        document.getElementById('rma-detail-actions').innerHTML = actionsHtml;

        // Test runs (already provided from the same API call)
        var runsEl = document.getElementById('rma-detail-runs');
        if (runs && runs.length > 0) {
            var html = '';
            for (var i = 0; i < runs.length; i++) {
                var run = runs[i];
                var statusClass = run.Status || 'error';
                html += '<div class="test-run-item ' + escapeHtml(statusClass) + '" onclick="App.toggleTestRunEvents(this, \'' + escapeHtml(run.ID) + '\')" style="cursor:pointer">';
                html += '<div class="test-run-info">';
                html += '<div class="test-run-script">' + escapeHtml(scriptLabel(run.ScriptName)) + '</div>';
                html += '<div class="test-run-meta">';
                html += '<span>' + formatDateTime(run.StartedAt) + '</span>';
                if (run.Summary) html += '<span>' + escapeHtml(run.Summary) + '</span>';
                html += '</div></div>';
                html += '<span class="test-run-status ' + escapeHtml(statusClass) + '">' + (statusClass.charAt(0).toUpperCase() + statusClass.slice(1)) + '</span>';
                html += '</div>';
                html += '<div class="test-run-events" id="run-events-' + escapeHtml(run.ID) + '" style="display:none"></div>';
            }
            runsEl.innerHTML = html;
        } else {
            runsEl.innerHTML = '<div class="empty-state">No test runs for this RMA</div>';
        }
    }

    function toggleTestRunEvents(el, testRunId) {
        var eventsDiv = document.getElementById('run-events-' + testRunId);
        if (!eventsDiv) return;
        if (eventsDiv.style.display !== 'none') {
            eventsDiv.style.display = 'none';
            return;
        }
        eventsDiv.style.display = 'block';
        if (eventsDiv.dataset.loaded) return;
        eventsDiv.innerHTML = '<div class="empty-state">Loading...</div>';
        api('GET', '/test-runs/' + encodeURIComponent(testRunId) + '/events', null, function(err, data) {
            if (err || !Array.isArray(data) || data.length === 0) {
                eventsDiv.innerHTML = '<div class="empty-state">No events</div>';
                return;
            }
            var html = '<table class="data-table"><thead><tr><th>Time</th><th>Event</th><th>Employee</th><th>Description</th></tr></thead><tbody>';
            for (var i = 0; i < data.length; i++) {
                var e = data[i];
                html += '<tr>';
                html += '<td class="timestamp">' + formatDateTime(e.Timestamp) + '</td>';
                html += '<td class="mono">' + escapeHtml(e.EventType) + '</td>';
                html += '<td>' + escapeHtml(e.EmployeeID || '--') + '</td>';
                html += '<td>' + escapeHtml(e.Reason || '--') + '</td>';
                html += '</tr>';
            }
            html += '</tbody></table>';
            eventsDiv.innerHTML = html;
            eventsDiv.dataset.loaded = '1';
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
        showConfirm(
            'Close RMA',
            'This will generate final artifacts and export to the file server. Continue?',
            'Close RMA', 'btn-danger',
            function(confirmed) {
                if (!confirmed) return;
                api('POST', '/rmas/' + encodeURIComponent(id) + '/close', null, function(err) {
                    if (err) {
                        showToast('Failed to close RMA: ' + err.message, 'error');
                        return;
                    }
                    loadRMADetail(id);
                });
            }
        );
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

    // Tap outside modal overlay to close
    document.addEventListener('click', function(e) {
        if (e.target.classList.contains('modal-overlay') && e.target.classList.contains('active')) {
            // For confirm dialog, trigger cancel
            if (e.target.id === 'confirm-dialog') {
                var cancelBtn = document.getElementById('confirm-dialog-cancel');
                if (cancelBtn && cancelBtn.onclick) cancelBtn.onclick();
                return;
            }
            e.target.classList.remove('active');
        }
    });

    // =================================================================
    // Custom Confirm Dialog (replaces native confirm())
    // =================================================================
    function showConfirm(title, message, okLabel, okClass, callback) {
        document.getElementById('confirm-dialog-title').textContent = title;
        document.getElementById('confirm-dialog-message').textContent = message;
        var okBtn = document.getElementById('confirm-dialog-ok');
        okBtn.textContent = okLabel || 'OK';
        okBtn.className = 'btn ' + (okClass || 'btn-primary');

        var cancelBtn = document.getElementById('confirm-dialog-cancel');

        function cleanup() {
            okBtn.onclick = null;
            cancelBtn.onclick = null;
            closeModal('confirm-dialog');
        }

        okBtn.onclick = function() { cleanup(); callback(true); };
        cancelBtn.onclick = function() { cleanup(); callback(false); };
        openModal('confirm-dialog');
    }

    // =================================================================
    // Toast Notifications (replaces native alert())
    // =================================================================
    function showToast(message, type) {
        type = type || 'error';
        var container = document.getElementById('toast-container');
        var toast = document.createElement('div');
        toast.className = 'toast ' + type;
        toast.textContent = message;
        toast.onclick = function() { dismissToast(toast); };
        container.appendChild(toast);
        setTimeout(function() { dismissToast(toast); }, 5000);
    }

    function dismissToast(el) {
        if (!el || !el.parentNode) return;
        el.style.animation = 'toast-out 0.3s ease-in forwards';
        setTimeout(function() { if (el.parentNode) el.parentNode.removeChild(el); }, 300);
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
                s.Status = 'online';
                s.UptimeSeconds = payload.uptime_seconds;
                s.FreeHeap = payload.free_heap;
                s.WifiRSSI = payload.wifi_rssi;
                s.FirmwareVersion = payload.firmware_version;
                if (payload.device_types) s.DeviceTypes = payload.device_types;
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
        if (!payload || !payload.station_instance) return;
        state.stationStates[payload.station_instance] = payload;

        if (state.currentView === 'stations') renderStationGrid();
        if (state.currentView === 'station-detail' && state.detailStation === payload.station_instance) {
            renderStationDetail(payload.station_instance);
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
            document.getElementById('detail-temp-1st').textContent = formatTemp(temps.first_stage);
            document.getElementById('detail-temp-2nd').textContent = formatTemp(temps.second_stage);

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
        if (state.currentView !== 'station-detail') return;
        if (payload.station_instance !== state.detailStation) return;

        var tbody = document.getElementById('detail-events-tbody');
        if (!tbody) return;

        // Clear "No events" placeholder if present
        var empty = tbody.querySelector('.empty-state');
        if (empty) tbody.innerHTML = '';

        var tr = document.createElement('tr');
        tr.innerHTML = '<td class="timestamp">' + formatDateTime(payload.timestamp) + '</td>' +
            '<td class="mono event-' + escapeHtml(payload.event_type) + '">' + escapeHtml(payload.event_type) + '</td>' +
            '<td>' + escapeHtml(payload.employee_id || '--') + '</td>' +
            '<td>' + escapeHtml(payload.description || payload.reason || payload.summary || '--') + '</td>';
        tbody.appendChild(tr);

        // Auto-scroll to bottom
        var scrollable = tbody.closest('.panel-body');
        if (scrollable) scrollable.scrollTop = scrollable.scrollHeight;
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

    // Initialize theme from localStorage
    initTheme();

    // Auto-login with pre-filled credentials
    login();

    // Handle enter key on login
    document.addEventListener('keydown', function(e) {
        if (e.key === 'Enter') {
            if (state.currentView === 'login') login();
            if (document.getElementById('manual-command') === document.activeElement) sendManualCommand();
        }
        if (e.key === 'Escape') {
            if (state.currentView === 'station-detail') showView('stations');
            else if (state.currentView === 'rma-detail') showView('rma-list');
            else if (state.currentView === 'rma-list' || state.currentView === 'rma-new') showView('stations');
        }
    });

    // =================================================================
    // Swipe-right gesture for back navigation
    // =================================================================
    (function() {
        var touchStartX = 0;
        var touchStartY = 0;
        var touchStartTime = 0;

        document.addEventListener('touchstart', function(e) {
            var touch = e.touches[0];
            touchStartX = touch.clientX;
            touchStartY = touch.clientY;
            touchStartTime = Date.now();
        }, { passive: true });

        document.addEventListener('touchend', function(e) {
            var touch = e.changedTouches[0];
            var dx = touch.clientX - touchStartX;
            var dy = Math.abs(touch.clientY - touchStartY);
            var dt = Date.now() - touchStartTime;

            // Trigger if: started near left edge, swiped right >80px,
            // minimal vertical drift, and quick gesture
            if (touchStartX < 40 && dx > 80 && dy < 50 && dt < 400) {
                if (state.currentView === 'station-detail') showView('stations');
                else if (state.currentView === 'rma-detail') showView('rma-list');
                else if (state.currentView === 'rma-list' || state.currentView === 'rma-new') showView('stations');
            }
        }, { passive: true });
    })();

    // =================================================================
    // Public API
    // =================================================================
    return {
        showView: showView,
        login: login,
        logout: logout,
        openStation: openStation,
        startTest: startTest,
        selectRMAForTest: selectRMAForTest,
        selectScriptForTest: selectScriptForTest,
        backToRMASelect: backToRMASelect,
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
        toggleTestRunEvents: toggleTestRunEvents,
        closeModal: closeModal,
        downloadArtifact: downloadArtifact,
        downloadPDF: downloadPDF,
        setTempWindow: setTempWindow,
        exportTempCSV: exportTempCSV,
        toggleTheme: toggleTheme
    };
})();
