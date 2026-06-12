document.documentElement.classList.add('dark');

let selectedFile = null;

function toggleTheme() {
    const html = document.documentElement;
    html.classList.toggle('dark');
    const icon = document.getElementById('theme-icon');
    icon.classList.toggle('fa-moon', html.classList.contains('dark'));
    icon.classList.toggle('fa-sun', !html.classList.contains('dark'));
}

// Drag-and-drop + file input
const dropZone = document.getElementById('drop-zone');
const fileInput = document.getElementById('file-input');
dropZone.addEventListener('click', () => fileInput.click());
dropZone.addEventListener('dragover', e => { e.preventDefault(); dropZone.classList.add('border-emerald-500'); });
dropZone.addEventListener('dragleave', () => dropZone.classList.remove('border-emerald-500'));
dropZone.addEventListener('drop', e => {
    e.preventDefault();
    dropZone.classList.remove('border-emerald-500');
    if (e.dataTransfer.files.length) handleFile(e.dataTransfer.files[0]);
});
fileInput.addEventListener('change', e => { if (e.target.files.length) handleFile(e.target.files[0]); });

function handleFile(file) {
    selectedFile = file;
    document.getElementById('selected-file-name').textContent = file.name;
    document.getElementById('file-name').classList.remove('hidden');
    document.getElementById('file-name').classList.add('flex');
}

function clearFile(e) {
    e.stopImmediatePropagation();
    selectedFile = null;
    document.getElementById('file-name').classList.add('hidden');
    document.getElementById('file-name').classList.remove('flex');
    document.getElementById('file-input').value = '';
}

function setQuickSymbol(sym) {
    document.getElementById('symbol-input').value = sym;
}

const RECENT_KEY = 'oi-recent-symbols';

function saveRecentSymbol(sym) {
    if (!sym) return;
    let recent = JSON.parse(localStorage.getItem(RECENT_KEY) || '[]');
    recent = [sym, ...recent.filter(s => s !== sym)].slice(0, 6);
    localStorage.setItem(RECENT_KEY, JSON.stringify(recent));
    renderRecentSymbols();
}

function renderRecentSymbols() {
    const container = document.getElementById('recent-symbols');
    if (!container) return;
    const recent = JSON.parse(localStorage.getItem(RECENT_KEY) || '[]');
    container.innerHTML = recent.length
        ? recent.map(sym => `<div class="cursor-pointer px-3 py-1 text-sm bg-slate-800 border border-slate-600 hover:bg-slate-700 hover:border-emerald-600 transition-all rounded-2xl" onclick="setQuickSymbol('${sym}');document.getElementById('analyze-symbol').value='${sym}';analyzeSymbol()">${sym}</div>`).join('')
        : '<span class="text-slate-500 text-xs px-2">No recent yet</span>';
}

async function uploadFile() {
    const symbol = document.getElementById('symbol-input').value.trim().toUpperCase();
    const statusEl = document.getElementById('upload-status');
    if (!selectedFile) { statusEl.innerHTML = `<span class="text-red-400">Select a file first.</span>`; return; }
    if (!symbol) { statusEl.innerHTML = `<span class="text-amber-400">Symbol is required.</span>`; return; }

    statusEl.innerHTML = `<span class="text-emerald-400"><i class="fa-solid fa-spinner fa-spin mr-2"></i>Uploading ${symbol}...</span>`;

    const fd = new FormData();
    fd.append('options_file', selectedFile);
    fd.append('symbol', symbol);

    try {
        const res = await fetch('/upload-excel', { method: 'POST', body: fd });
        const result = await res.json();
        if (!res.ok) throw new Error(result.error || 'Upload failed');
        const usedSym = result.symbols?.[0] || symbol;
        document.getElementById('analyze-symbol').value = usedSym;
        saveRecentSymbol(usedSym);
        statusEl.innerHTML = `<span class="text-emerald-400">✓ ${result.records} records loaded. Running analysis...</span>`;
        analyzeSymbol();
    } catch (e) {
        statusEl.innerHTML = `<span class="text-red-400">Upload failed: ${e.message}</span>`;
    }
}

async function analyzeSymbol() {
    const symbol = document.getElementById('analyze-symbol').value.trim().toUpperCase()
        || document.getElementById('symbol-input').value.trim().toUpperCase();
    if (!symbol) { alert('Enter a symbol'); return; }

    document.getElementById('results-placeholder').classList.add('hidden');
    const content = document.getElementById('results-content');
    content.classList.remove('hidden');
    content.innerHTML = `<div class="p-8 flex flex-col items-center justify-center h-[420px]">
        <i class="fa-solid fa-spinner fa-spin text-4xl text-emerald-400 mb-4"></i>
        <div class="font-medium">Analyzing ${symbol}...</div></div>`;

    try {
        const res = await fetch(`/analyse?symbol=${symbol}`);
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'No data');
        renderResults(data, symbol);
    } catch (e) {
        content.innerHTML = `<div class="p-8 text-center">
            <i class="fa-solid fa-exclamation-triangle text-red-400 text-3xl mb-3"></i>
            <div class="font-medium">${e.message}</div>
            <button onclick="document.getElementById('results-placeholder').classList.remove('hidden');document.getElementById('results-content').classList.add('hidden')"
                    class="mt-5 text-sm px-4 py-2 bg-slate-800 rounded-2xl">Back</button></div>`;
    }
}

function renderResults(data, symbol) {
    saveRecentSymbol(symbol);
    const sigs = data.signals || [];
    const m = data.metrics || {};
    const expiries = data.expiries || [];

    const buyCount   = sigs.filter(s => s.action === 'BUY').length;
    const sellCount  = sigs.filter(s => s.action === 'SELL').length;
    const watchCount = sigs.filter(s => s.action === 'WATCH').length;

    // Bias card from server-computed metrics
    const biasColor = { Bullish: 'emerald', Bearish: 'red', Neutral: 'amber' }[m.bias] || 'amber';
    const biasIcon  = { Bullish: 'fa-arrow-trend-up', Bearish: 'fa-arrow-trend-down', Neutral: 'fa-arrows-left-right' }[m.bias] || 'fa-arrows-left-right';
    const biasHTML = `<div class="flex gap-4 items-start">
        <div class="text-${biasColor}-400 mt-0.5"><i class="fa-solid ${biasIcon} text-2xl"></i></div>
        <div>
            <div class="font-semibold text-${biasColor}-300">${m.bias || 'Neutral'}</div>
            <div class="text-sm text-${biasColor}-400 mt-0.5">${m.bias_reason || ''}</div>
        </div></div>`;

    // Metrics strip
    const fmt2 = v => (v != null ? v.toFixed(2) : '—');
    const metricsHTML = `
        <div class="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-6">
            <div class="bg-slate-800 border border-slate-700 rounded-2xl p-3">
                <div class="text-xs text-slate-400 mb-1">PCR</div>
                <div class="text-xl font-semibold font-mono">${fmt2(m.pcr)}</div>
                <div class="text-xs text-slate-500 mt-0.5">${m.total_pe_oi?.toLocaleString()} PE / ${m.total_ce_oi?.toLocaleString()} CE</div>
            </div>
            <div class="bg-slate-800 border border-slate-700 rounded-2xl p-3">
                <div class="text-xs text-slate-400 mb-1">Max Pain</div>
                <div class="text-xl font-semibold font-mono">${m.max_pain || '—'}</div>
                <div class="text-xs text-slate-500 mt-0.5">expiry magnet</div>
            </div>
            <div class="bg-slate-800 border border-slate-700 rounded-2xl p-3">
                <div class="text-xs text-slate-400 mb-1">IV Skew</div>
                <div class="text-xl font-semibold font-mono ${(m.iv_skew||0) > 0 ? 'text-emerald-400' : (m.iv_skew||0) < 0 ? 'text-red-400' : ''}">${fmt2(m.iv_skew)}</div>
                <div class="text-xs text-slate-500 mt-0.5">CE IV − PE IV</div>
            </div>
            <div class="bg-slate-800 border border-slate-700 rounded-2xl p-3">
                <div class="text-xs text-slate-400 mb-1">Signals</div>
                <div class="text-xl font-semibold font-mono">${sigs.length}</div>
                <div class="text-xs text-slate-500 mt-0.5">${buyCount}B / ${sellCount}S / ${watchCount}W</div>
            </div>
        </div>`;

    // Top strikes
    const topStrikesHTML = (m.top_ce_strikes?.length || m.top_pe_strikes?.length) ? `
        <div class="grid grid-cols-2 gap-3 mb-6">
            <div class="bg-slate-800 border border-slate-700 rounded-2xl p-3">
                <div class="text-xs text-emerald-400 font-medium mb-2">TOP CE OI STRIKES</div>
                ${(m.top_ce_strikes||[]).map(s => `<div class="font-mono text-sm py-0.5">${s}</div>`).join('')}
            </div>
            <div class="bg-slate-800 border border-slate-700 rounded-2xl p-3">
                <div class="text-xs text-red-400 font-medium mb-2">TOP PE OI STRIKES</div>
                ${(m.top_pe_strikes||[]).map(s => `<div class="font-mono text-sm py-0.5">${s}</div>`).join('')}
            </div>
        </div>` : '';

    // Expiry OI bar chart (only when multi-expiry)
    const expiryChartHTML = expiries.length > 1 ? `
        <div class="bg-slate-800 border border-slate-700 rounded-2xl p-4 mb-6">
            <div class="text-xs font-medium text-slate-400 mb-3">OI BY EXPIRY</div>
            <canvas id="expiry-chart" height="80"></canvas>
        </div>` : '';

    // Signals table
    const tableRows = sigs.slice(0, 20).map(s => {
        const cls = s.action === 'BUY' ? 'bg-emerald-700 text-emerald-200' : s.action === 'SELL' ? 'bg-red-700 text-red-200' : 'bg-amber-700 text-amber-200';
        return `<tr class="hover:bg-slate-800">
            <td class="px-3 py-2.5 font-mono text-sm">${s.strike_price}</td>
            <td class="px-3 py-2.5"><span class="text-xs px-2 py-0.5 bg-slate-700 rounded font-medium">${s.option_type}</span></td>
            <td class="px-3 py-2.5 text-xs text-slate-400">${s.expiry || ''}</td>
            <td class="px-3 py-2.5"><span class="signal-badge ${cls}">${s.action}</span></td>
            <td class="px-3 py-2.5 text-xs font-medium">${s.confidence}</td>
            <td class="px-3 py-2.5 text-xs text-slate-400">${s.rationale}</td>
        </tr>`;
    }).join('');

    document.getElementById('results-content').innerHTML = `
        <div class="p-6">
            <div class="flex justify-between items-end mb-5">
                <div>
                    <div class="text-emerald-400 text-xs tracking-widest">ANALYSIS</div>
                    <div class="title-font text-5xl font-semibold text-white">${symbol}</div>
                </div>
                <div class="text-right text-xs text-slate-500">${data.cached ? 'CACHED' : 'FRESH'}</div>
            </div>
            ${metricsHTML}
            <div class="insight-card rounded-2xl border p-4 mb-6">${biasHTML}</div>
            ${topStrikesHTML}
            ${expiryChartHTML}
            <div>
                <div class="text-xs font-medium text-slate-400 mb-2">SIGNALS (${sigs.length})</div>
                <div class="bg-slate-800 border border-slate-700 rounded-2xl overflow-hidden">
                    <table class="w-full text-sm">
                        <thead><tr class="text-xs bg-slate-900 text-slate-400">
                            <th class="px-3 py-2.5 text-left">Strike</th>
                            <th class="px-3 py-2.5 text-left">Type</th>
                            <th class="px-3 py-2.5 text-left">Expiry</th>
                            <th class="px-3 py-2.5 text-left">Action</th>
                            <th class="px-3 py-2.5 text-left">Conf.</th>
                            <th class="px-3 py-2.5 text-left">Rationale</th>
                        </tr></thead>
                        <tbody class="divide-y divide-slate-700">
                            ${tableRows || '<tr><td colspan="6" class="px-4 py-6 text-center text-slate-400">No significant OI changes above threshold</td></tr>'}
                        </tbody>
                    </table>
                </div>
            </div>
        </div>`;

    if (expiries.length > 1) renderExpiryChart(expiries);
}

function renderExpiryChart(expiries) {
    const canvas = document.getElementById('expiry-chart');
    if (!canvas) return;
    if (window._expiryChart) window._expiryChart.destroy();
    window._expiryChart = new Chart(canvas, {
        type: 'bar',
        data: {
            labels: expiries.map(e => e.expiry),
            datasets: [
                { label: 'CE OI', data: expiries.map(e => e.total_ce_oi), backgroundColor: '#10b981bb' },
                { label: 'PE OI', data: expiries.map(e => e.total_pe_oi), backgroundColor: '#ef4444bb' },
            ]
        },
        options: {
            responsive: true,
            scales: {
                x: { ticks: { color: '#64748b', font: { size: 10 } }, grid: { color: '#1e293b' } },
                y: { ticks: { color: '#64748b', font: { size: 10 } }, grid: { color: '#1e293b' } }
            },
            plugins: { legend: { labels: { color: '#94a3b8', boxWidth: 12, font: { size: 11 } } } }
        }
    });
}

function quickDemo() {
    renderResults({
        cached: false,
        metrics: { pcr: 1.42, max_pain: 23000, iv_skew: -1.2, bias: 'Bullish', bias_reason: 'PCR 1.42 indicates heavy put writing; institutions are selling puts (expecting support).', total_ce_oi: 8200000, total_pe_oi: 11640000, top_ce_strikes: [23200, 23500, 23800], top_pe_strikes: [22500, 22000, 21500] },
        expiries: [
            { expiry: '19-Jun-2026', total_ce_oi: 4200000, total_pe_oi: 5600000, pcr: 1.33 },
            { expiry: '26-Jun-2026', total_ce_oi: 2500000, total_pe_oi: 3800000, pcr: 1.52 },
            { expiry: '31-Jul-2026', total_ce_oi: 1500000, total_pe_oi: 2240000, pcr: 1.49 },
        ],
        signals: [
            { strike_price: 23500, option_type: 'CE', expiry: '19-Jun-2026', action: 'WATCH', rationale: 'CE OI build-up (+195%) — call writing resistance.', confidence: 'MEDIUM' },
            { strike_price: 22500, option_type: 'PE', expiry: '19-Jun-2026', action: 'BUY',   rationale: 'Heavy put writing (+519%) — institutions expect support (bullish).', confidence: 'MEDIUM' },
            { strike_price: 21500, option_type: 'PE', expiry: '26-Jun-2026', action: 'BUY',   rationale: 'PE OI build-up (+179%) — bullish put writing.', confidence: 'MEDIUM' },
        ]
    }, 'DEMO');
}

function init() {
    renderRecentSymbols();
    // Simple health dot — no reconnect theater
    fetch('/healthz').then(r => {
        const dot = document.getElementById('health-dot');
        const text = document.getElementById('health-text');
        if (r.ok) {
            dot.className = 'w-2.5 h-2.5 bg-emerald-400 rounded-full animate-pulse';
            text.textContent = 'Connected';
            text.className = 'text-emerald-400 text-xs font-medium';
        } else throw new Error();
    }).catch(() => {
        document.getElementById('health-dot').className = 'w-2.5 h-2.5 bg-red-500 rounded-full';
        document.getElementById('health-text').textContent = 'Offline';
        document.getElementById('health-text').className = 'text-red-400 text-xs font-medium';
    });
}

document.addEventListener('keydown', e => {
    if (e.key === '/' && document.activeElement.tagName === 'BODY') {
        e.preventDefault();
        document.getElementById('symbol-input').focus();
    }
});

window.onload = init;
