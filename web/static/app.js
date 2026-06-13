// Global State
let currentAnalysisData = null;
let activeTab = 'dashboard';
let selectedFile = null;

// Chart references for proper cleanup
let expiryChartRef = null;
let strikeChartRef = null;

// Initialize
document.addEventListener('DOMContentLoaded', () => {
    init();
});

function init() {
    renderRecentSymbols();
    checkHealth();
    switchTab('dashboard');
}

// Health Check
function checkHealth() {
    fetch('/healthz')
        .then(res => {
            const dot = document.getElementById('health-dot');
            const text = document.getElementById('health-text');
            if (res.ok) {
                if (dot) dot.className = 'w-2 h-2 bg-emerald-400 rounded-full animate-pulse';
                if (text) {
                    text.textContent = 'Connected';
                    text.className = 'text-emerald-400 text-xs font-semibold';
                }
            } else {
                throw new Error();
            }
        })
        .catch(() => {
            const dot = document.getElementById('health-dot');
            const text = document.getElementById('health-text');
            if (dot) dot.className = 'w-2 h-2 bg-rose-500 rounded-full';
            if (text) {
                text.textContent = 'Offline';
                text.className = 'text-rose-400 text-xs font-semibold';
            }
        });
}

// Tab switcher
function switchTab(tabId) {
    activeTab = tabId;
    
    // Toggle tab header styles
    const tabs = ['dashboard', 'matrix', 'charts'];
    tabs.forEach(t => {
        const btn = document.getElementById(`tab-${t}`);
        const panel = document.getElementById(`panel-${t}`);
        if (!btn || !panel) return;
        
        if (t === tabId) {
            btn.className = 'px-4 py-2 text-xs font-bold rounded-xl transition-all duration-200 bg-emerald-500 text-slate-950 shadow-md shadow-emerald-500/10';
            panel.classList.remove('hidden');
        } else {
            btn.className = 'px-4 py-2 text-xs font-bold rounded-xl transition-all duration-200 text-slate-400 hover:text-slate-100 hover:bg-slate-800/40';
            panel.classList.add('hidden');
        }
    });

    // Re-render components that require dimension calculations on tab switch
    if (tabId === 'charts') {
        renderCharts();
    }
}

// Set quick symbols
function setQuickSymbol(sym) {
    document.getElementById('symbol-input').value = sym;
}

// Drag & Drop
const dropZone = document.getElementById('drop-zone');
const fileInput = document.getElementById('file-input');

if (dropZone && fileInput) {
    dropZone.addEventListener('click', () => fileInput.click());
    dropZone.addEventListener('dragover', e => { 
        e.preventDefault(); 
        dropZone.classList.add('border-emerald-500', 'bg-slate-900/60'); 
    });
    dropZone.addEventListener('dragleave', () => {
        dropZone.classList.remove('border-emerald-500', 'bg-slate-900/60');
    });
    dropZone.addEventListener('drop', e => {
        e.preventDefault();
        dropZone.classList.remove('border-emerald-500', 'bg-slate-900/60');
        if (e.dataTransfer.files.length) handleFile(e.dataTransfer.files[0]);
    });
    fileInput.addEventListener('change', e => { 
        if (e.target.files.length) handleFile(e.target.files[0]); 
    });
}

function handleFile(file) {
    selectedFile = file;
    const nameEl = document.getElementById('selected-file-name');
    if (nameEl) nameEl.textContent = file.name;
    const fileCard = document.getElementById('file-name');
    if (fileCard) {
        fileCard.classList.remove('hidden');
        fileCard.classList.add('flex');
    }
}

function clearFile(e) {
    if (e) e.stopPropagation();
    selectedFile = null;
    const fileCard = document.getElementById('file-name');
    if (fileCard) {
        fileCard.classList.add('hidden');
        fileCard.classList.remove('flex');
    }
    const fileInput = document.getElementById('file-input');
    if (fileInput) fileInput.value = '';
}

// Local Storage for Recent Symbols
const RECENT_KEY = 'oi-recent-symbols';
function saveRecentSymbol(sym) {
    if (!sym) return;
    let recent = JSON.parse(localStorage.getItem(RECENT_KEY) || '[]');
    recent = [sym, ...recent.filter(s => s !== sym)].slice(0, 5);
    localStorage.setItem(RECENT_KEY, JSON.stringify(recent));
    renderRecentSymbols();
}

function renderRecentSymbols() {
    const container = document.getElementById('recent-symbols');
    if (!container) return;
    const recent = JSON.parse(localStorage.getItem(RECENT_KEY) || '[]');
    container.innerHTML = recent.length
        ? recent.map(sym => `
            <div onclick="quickAnalyzeRecent('${sym}')" 
                 class="cursor-pointer px-3 py-1.5 bg-slate-900 border border-slate-800 hover:border-emerald-500/50 hover:bg-slate-800/60 rounded-xl text-slate-300 hover:text-white text-xs font-semibold transition-all">
                ${sym}
            </div>`).join('')
        : '<span class="text-slate-600 text-xs italic px-1">No recent searches</span>';
}

function quickAnalyzeRecent(sym) {
    setQuickSymbol(sym);
    triggerAnalysis(sym);
}

// Live Fetch from NSE
async function fetchNSE() {
    const symbol = document.getElementById('symbol-input').value.trim().toUpperCase();
    if (!symbol) {
        showStatus('Please enter a symbol', 'rose');
        return;
    }

    setLoadingState(true, `Fetching live ${symbol} option chain from NSE...`);

    try {
        const res = await fetch(`/fetch-nse?symbol=${symbol}`);
        const result = await res.json();
        if (!res.ok) throw new Error(result.error || 'Live fetch failed');
        
        saveRecentSymbol(symbol);
        showStatus(`✓ Loaded ${result.records} rows for ${symbol} at Spot ${result.spot_price.toFixed(2)}`, 'emerald');
        await triggerAnalysis(symbol);
    } catch (err) {
        setLoadingState(false);
        showStatus(err.message, 'rose');
        
        // Show helpful tip to manually download CSV if blocked by NSE bot detection
        const errorContent = `
            <div class="p-8 text-center my-auto flex-1 flex flex-col justify-center items-center">
                <div class="w-16 h-16 bg-rose-950/60 border border-rose-900 rounded-3xl flex items-center justify-center mb-5 shadow-lg">
                    <i class="fa-solid fa-triangle-exclamation text-2xl text-rose-400"></i>
                </div>
                <h3 class="text-xl font-bold text-white">NSE Live Fetch Blocked</h3>
                <p class="text-slate-400 text-sm mt-2 max-w-md">
                    NSE uses aggressive bot-protection. Please manually download the CSV from <a href="https://www.nseindia.com/option-chain" target="_blank" class="text-emerald-400 hover:underline">NSE Option Chain</a> and upload it using the card on the left.
                </p>
                <div class="mt-4 text-xs bg-slate-950 border border-slate-850 p-2.5 rounded-xl font-mono text-slate-500">
                    Error: ${err.message}
                </div>
            </div>`;
        document.getElementById('results-placeholder').classList.add('hidden');
        document.getElementById('results-loader').classList.add('hidden');
        const content = document.getElementById('results-content');
        content.classList.remove('hidden');
        content.innerHTML = errorContent;
    }
}

// File Upload
async function uploadFile() {
    const symbol = document.getElementById('symbol-input').value.trim().toUpperCase();
    if (!selectedFile) {
        showStatus('Please select a file first', 'rose');
        return;
    }
    if (!symbol) {
        showStatus('Please enter a symbol', 'rose');
        return;
    }

    showStatus(`Uploading options sheet for ${symbol}...`, 'emerald');
    const fd = new FormData();
    fd.append('options_file', selectedFile);
    fd.append('symbol', symbol);

    try {
        const res = await fetch('/upload-excel', { method: 'POST', body: fd });
        const result = await res.json();
        if (!res.ok) throw new Error(result.error || 'Upload failed');
        
        const usedSym = result.symbols?.[0] || symbol;
        saveRecentSymbol(usedSym);
        showStatus(`✓ Sheet processed. Loaded ${result.records} rows.`, 'emerald');
        clearFile();
        await triggerAnalysis(usedSym);
    } catch (err) {
        showStatus(err.message, 'rose');
    }
}

// Trigger analysis API
async function triggerAnalysis(symbol) {
    setLoadingState(true, `Analyzing Open Interest for ${symbol}...`);
    try {
        const res = await fetch(`/analyse?symbol=${symbol}`);
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'Analysis failed');

        currentAnalysisData = data;
        setLoadingState(false);
        renderResults();
    } catch (err) {
        setLoadingState(false);
        showStatus(err.message, 'rose');
    }
}

function setLoadingState(loading, text = '') {
    const placeholder = document.getElementById('results-placeholder');
    const loader = document.getElementById('results-loader');
    const content = document.getElementById('results-content');
    const loaderText = document.getElementById('loader-text');

    if (loading) {
        if (placeholder) placeholder.classList.add('hidden');
        if (content) content.classList.add('hidden');
        if (loader) loader.classList.remove('hidden');
        if (loaderText) loaderText.textContent = text;
    } else {
        if (loader) loader.classList.add('hidden');
    }
}

function showStatus(msg, type) {
    const el = document.getElementById('upload-status');
    if (!el) return;
    
    if (type === 'emerald') {
        el.innerHTML = `<span class="text-emerald-400 font-semibold"><i class="fa-solid fa-circle-check mr-1.5"></i>${msg}</span>`;
    } else if (type === 'rose') {
        el.innerHTML = `<span class="text-rose-400 font-semibold"><i class="fa-solid fa-circle-exclamation mr-1.5"></i>${msg}</span>`;
    } else {
        el.innerHTML = `<span class="text-slate-400">${msg}</span>`;
    }
}

// Render Results on the screen
function renderResults() {
    if (!currentAnalysisData) return;
    
    // Clear strategy builder basket for new symbol
    if (typeof clearBasket === 'function') {
        clearBasket(null, false);
    }
    
    const d = currentAnalysisData;
    const placeholder = document.getElementById('results-placeholder');
    const content = document.getElementById('results-content');
    
    if (placeholder) placeholder.classList.add('hidden');
    if (content) content.classList.remove('hidden');

    // Title & spot
    const symbolTitle = document.getElementById('result-symbol-title');
    if (symbolTitle) symbolTitle.textContent = d.symbol;
    const dataFreshness = document.getElementById('data-freshness');
    if (dataFreshness) dataFreshness.textContent = d.cached ? 'CACHED SUMMARY' : 'REALTIME RESULTS';
    
    const spotBadge = document.getElementById('spot-price-badge');
    if (spotBadge) {
        if (d.spot_price) {
            spotBadge.innerHTML = `<i class="fa-solid fa-circle text-[9px] text-emerald-400 animate-pulse mr-1.5"></i>SPOT: <span class="font-bold text-white">${d.spot_price.toFixed(2)}</span>`;
            spotBadge.classList.remove('hidden');
        } else {
            spotBadge.classList.add('hidden');
        }
    }

    // AI report resets
    const aiReportBody = document.getElementById('ai-report-body');
    if (aiReportBody) {
        if (d.ai_report) {
            aiReportBody.innerHTML = renderMarkdown(d.ai_report);
        } else {
            aiReportBody.innerHTML = `<span class="text-slate-500 italic">No AI commentary loaded. Click "Generate AI Commentary" to fetch comprehensive market commentary.</span>`;
        }
    }

    // Statistics Panels
    renderStats(d.metrics, d.signals);

    // Bias Card
    renderBias(d.metrics);

    // Signals Table
    renderSignalsTable(d.signals);

    // Populate Expiry filter select in matrix tab
    populateMatrixExpiries(d.records);

    // Render Matrix Table
    renderMatrixTable();

    // Reset tab to dashboard
    switchTab('dashboard');
}

function renderStats(m, sigs) {
    const buyCount = sigs.filter(s => s.action === 'BUY').length;
    const sellCount = sigs.filter(s => s.action === 'SELL').length;
    const watchCount = sigs.filter(s => s.action === 'WATCH').length;

    const container = document.getElementById('metrics-container');
    container.innerHTML = `
        <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div class="bg-slate-900 border border-slate-800 rounded-2xl p-4 shadow-sm">
                <div class="text-[10px] font-bold text-slate-500 uppercase tracking-wider mb-1">Put-Call Ratio</div>
                <div class="text-2xl font-extrabold font-mono text-white">${m.pcr.toFixed(2)}</div>
                <div class="text-[10px] text-slate-400 mt-1 font-mono">${m.total_pe_oi.toLocaleString()} PE / ${m.total_ce_oi.toLocaleString()} CE</div>
            </div>
            <div class="bg-slate-900 border border-slate-800 rounded-2xl p-4 shadow-sm">
                <div class="text-[10px] font-bold text-slate-500 uppercase tracking-wider mb-1">Max Pain Strike</div>
                <div class="text-2xl font-extrabold font-mono text-white">${m.max_pain.toLocaleString()}</div>
                <div class="text-[10px] text-slate-400 mt-1">Expiry Magnet Strike</div>
            </div>
            <div class="bg-slate-900 border border-slate-800 rounded-2xl p-4 shadow-sm">
                <div class="text-[10px] font-bold text-slate-500 uppercase tracking-wider mb-1">IV Skew</div>
                <div class="text-2xl font-extrabold font-mono ${m.iv_skew > 0 ? 'text-emerald-400' : m.iv_skew < 0 ? 'text-rose-400' : 'text-white'}">${m.iv_skew.toFixed(2)}</div>
                <div class="text-[10px] text-slate-400 mt-1 font-mono">CE IV − PE IV</div>
            </div>
            <div class="bg-slate-900 border border-slate-800 rounded-2xl p-4 shadow-sm">
                <div class="text-[10px] font-bold text-slate-500 uppercase tracking-wider mb-1">Trading Signals</div>
                <div class="text-2xl font-extrabold font-mono text-white">${sigs.length}</div>
                <div class="text-[10px] text-slate-400 mt-1 font-semibold">
                    <span class="text-emerald-400">${buyCount}B</span> / 
                    <span class="text-rose-400">${sellCount}S</span> / 
                    <span class="text-amber-400">${watchCount}W</span>
                </div>
            </div>
        </div>`;
}

function renderBias(m) {
    const color = { Bullish: 'emerald', Bearish: 'rose', Neutral: 'amber' }[m.bias] || 'amber';
    const icon = { Bullish: 'fa-arrow-trend-up', Bearish: 'fa-arrow-trend-down', Neutral: 'fa-arrows-left-right' }[m.bias] || 'fa-arrows-left-right';
    
    const container = document.getElementById('bias-card-container');
    container.innerHTML = `
        <div class="bg-slate-900 border border-slate-800 p-5 rounded-2xl flex items-start gap-4 shadow-sm">
            <div class="w-12 h-12 rounded-xl bg-${color}-950/80 border border-${color}-900 flex items-center justify-center text-${color}-400 shrink-0">
                <i class="fa-solid ${icon} text-xl"></i>
            </div>
            <div>
                <h4 class="font-bold text-white text-base">Sentiment Bias: <span class="text-${color}-400">${m.bias}</span></h4>
                <p class="text-xs text-slate-400 mt-1 leading-relaxed">${m.bias_reason}</p>
            </div>
        </div>`;
}

function renderSignalsTable(sigs) {
    const tbody = document.getElementById('signals-table-body');
    if (!sigs || sigs.length === 0) {
        tbody.innerHTML = `
            <tr>
                <td colspan="6" class="px-4 py-8 text-center text-slate-500 font-medium italic">
                    No option contracts exceeded the significant OI change threshold (20%).
                </td>
            </tr>`;
        return;
    }

    tbody.innerHTML = sigs.map(s => {
        let actionBadgeColor = '';
        if (s.action === 'BUY') actionBadgeColor = 'bg-emerald-950 text-emerald-400 border border-emerald-900';
        else if (s.action === 'SELL') actionBadgeColor = 'bg-rose-950 text-rose-400 border border-rose-900';
        else actionBadgeColor = 'bg-amber-950 text-amber-400 border border-amber-900';

        let confColor = { HIGH: 'text-emerald-400', MEDIUM: 'text-amber-400', LOW: 'text-slate-400' }[s.confidence] || 'text-slate-400';

        return `
            <tr class="hover:bg-slate-900/50 transition-colors font-mono">
                <td class="px-4 py-3.5 font-bold text-slate-200">${s.strike_price.toLocaleString()}</td>
                <td class="px-4 py-3.5">
                    <span class="px-2 py-0.5 rounded text-[10px] font-bold ${s.option_type === 'CE' ? 'bg-emerald-950 text-emerald-300' : 'bg-rose-950 text-rose-300'}">
                        ${s.option_type}
                    </span>
                </td>
                <td class="px-4 py-3.5 text-xs text-slate-400">${s.expiry}</td>
                <td class="px-4 py-3.5 text-center">
                    <span class="px-2.5 py-0.5 rounded-full text-[10px] font-bold tracking-wider ${actionBadgeColor}">
                        ${s.action}
                    </span>
                </td>
                <td class="px-4 py-3.5 text-center text-xs font-bold ${confColor}">${s.confidence}</td>
                <td class="px-4 py-3.5 text-xs font-sans text-slate-300 font-medium">${s.rationale}</td>
            </tr>`;
    }).join('');
}

// Option Chain Matrix Tab Functions
function populateMatrixExpiries(records) {
    const select = document.getElementById('matrix-expiry-select');
    if (!select || !records) return;

    // Collect unique expiries
    const expiries = [...new Set(records.map(r => r.expiry || 'Current Expiry'))].sort((a, b) => {
        const da = new Date(a);
        const db = new Date(b);
        if (isNaN(da) || isNaN(db)) return a.localeCompare(b);
        return da - db;
    });

    select.innerHTML = expiries.map(exp => `<option value="${exp}">${exp}</option>`).join('');
}

function renderMatrixTable() {
    if (!currentAnalysisData) return;
    const select = document.getElementById('matrix-expiry-select');
    const tbody = document.getElementById('matrix-table-body');
    if (!select || !tbody) return;

    const expiry = select.value;
    const records = currentAnalysisData.records || [];
    
    // Filter by selected expiry
    const filtered = records.filter(r => (r.expiry || 'Current Expiry') === expiry);

    // Group by strike
    const grouped = {};
    filtered.forEach(r => {
        if (!grouped[r.strike_price]) {
            grouped[r.strike_price] = { strike: r.strike_price };
        }
        grouped[r.strike_price][r.option_type] = r;
    });

    // Sort strikes
    const strikes = Object.keys(grouped).map(Number).sort((a, b) => a - b);
    
    // Estimate spot price
    const spot = currentAnalysisData.spot_price || 0;
    
    // Find ATM strike (closest to spot price)
    let atmStrike = 0;
    if (spot > 0 && strikes.length > 0) {
        let diff = Infinity;
        strikes.forEach(s => {
            if (Math.abs(s - spot) < diff) {
                diff = Math.abs(s - spot);
                atmStrike = s;
            }
        });
    }

    if (strikes.length === 0) {
        tbody.innerHTML = `<tr><td colspan="7" class="px-4 py-6 text-slate-500 italic">No option chain records for this expiry</td></tr>`;
        return;
    }

    tbody.innerHTML = strikes.map(s => {
        const item = grouped[s];
        const ce = item.CE || {};
        const pe = item.PE || {};

        const ceOI = ce.oi ? ce.oi.toLocaleString() : '—';
        const ceOIChange = ce.oi_change_pct ? `${ce.oi_change_pct > 0 ? '+' : ''}${ce.oi_change_pct.toFixed(1)}%` : '—';
        const ceLtp = ce.ltp ? ce.ltp.toFixed(2) : '—';
        
        const peOI = pe.oi ? pe.oi.toLocaleString() : '—';
        const peOIChange = pe.oi_change_pct ? `${pe.oi_change_pct > 0 ? '+' : ''}${pe.oi_change_pct.toFixed(1)}%` : '—';
        const peLtp = pe.ltp ? pe.ltp.toFixed(2) : '—';

        const isCeITM = spot > 0 && s < spot;
        const isPeITM = spot > 0 && s > spot;
        const ceBgClass = isCeITM ? 'bg-slate-900/70' : '';
        const peBgClass = isPeITM ? 'bg-slate-900/70' : '';

        const isATM = s === atmStrike;
        const atmClass = isATM ? 'bg-emerald-500/10 border-y border-emerald-500/30' : '';
        const strikeLabel = isATM ? `<span class="bg-emerald-500 text-slate-950 font-bold px-1.5 py-0.5 rounded text-[9px] tracking-wide animate-pulse">ATM</span><span class="block text-slate-100 font-bold text-xs mt-0.5">${s}</span>` : `<span class="text-slate-400 font-bold text-xs">${s}</span>`;

        const ceChangeColor = ce.oi_change_pct > 0 ? 'text-emerald-400' : ce.oi_change_pct < 0 ? 'text-rose-400' : 'text-slate-500';
        const peChangeColor = pe.oi_change_pct > 0 ? 'text-emerald-400' : pe.oi_change_pct < 0 ? 'text-rose-400' : 'text-slate-500';

        const ceLtpMarkup = ce.ltp ? `
            <div class="flex items-center justify-between h-full w-full">
                <span>${ce.ltp.toFixed(2)}</span>
                <div class="opacity-0 group-hover:opacity-100 absolute inset-0 bg-slate-900/90 flex items-center justify-center gap-1 transition-opacity duration-150">
                    <button onclick="addToBasket(${s}, 'CE', 'BUY', ${ce.ltp}, '${expiry}')" class="bg-emerald-500 hover:bg-emerald-400 text-slate-950 font-extrabold px-2 py-0.5 rounded text-[10px] transition-colors shadow-sm">B</button>
                    <button onclick="addToBasket(${s}, 'CE', 'SELL', ${ce.ltp}, '${expiry}')" class="bg-rose-500 hover:bg-rose-400 text-slate-950 font-extrabold px-2 py-0.5 rounded text-[10px] transition-colors shadow-sm">S</button>
                </div>
            </div>` : '—';

        const peLtpMarkup = pe.ltp ? `
            <div class="flex items-center justify-between h-full w-full">
                <span>${pe.ltp.toFixed(2)}</span>
                <div class="opacity-0 group-hover:opacity-100 absolute inset-0 bg-slate-900/90 flex items-center justify-center gap-1 transition-opacity duration-150">
                    <button onclick="addToBasket(${s}, 'PE', 'BUY', ${pe.ltp}, '${expiry}')" class="bg-emerald-500 hover:bg-emerald-400 text-slate-950 font-extrabold px-2 py-0.5 rounded text-[10px] transition-colors shadow-sm">B</button>
                    <button onclick="addToBasket(${s}, 'PE', 'SELL', ${pe.ltp}, '${expiry}')" class="bg-rose-500 hover:bg-rose-400 text-slate-950 font-extrabold px-2 py-0.5 rounded text-[10px] transition-colors shadow-sm">S</button>
                </div>
            </div>` : '—';

        return `
            <tr class="hover:bg-slate-900/40 border-b border-slate-900/60 ${atmClass}">
                <td class="px-4 py-2 text-left text-slate-300 text-xs font-mono ${ceBgClass}">${ceOI}</td>
                <td class="px-4 py-2 font-mono text-xs ${ceChangeColor} ${ceBgClass}">${ceOIChange}</td>
                <td class="px-4 py-2 border-r border-slate-850/60 text-slate-400 text-xs font-mono ${ceBgClass} relative group select-none cursor-pointer min-w-[90px] h-[34px]">${ceLtpMarkup}</td>
                
                <td class="px-4 py-2 bg-slate-900/40 font-mono align-middle select-all">${strikeLabel}</td>
                
                <td class="px-4 py-2 border-l border-slate-850/60 text-slate-400 text-xs font-mono ${peBgClass} relative group select-none cursor-pointer min-w-[90px] h-[34px]">${peLtpMarkup}</td>
                <td class="px-4 py-2 font-mono text-xs ${peChangeColor} ${peBgClass}">${peOIChange}</td>
                <td class="px-4 py-2 text-right text-slate-300 text-xs font-mono ${peBgClass}">${peOI}</td>
            </tr>`;
    }).join('');
}

// Generate AI Commentary
async function generateAIReport() {
    if (!currentAnalysisData) return;
    const btn = document.getElementById('btn-ai-report');
    const spinner = document.getElementById('ai-spinner');
    const reportBody = document.getElementById('ai-report-body');

    btn.disabled = true;
    spinner.classList.add('fa-spin');
    reportBody.innerHTML = `<span class="text-emerald-400"><i class="fa-solid fa-spinner fa-spin mr-2"></i>AI is reviewing Open Interest signals and drafting report...</span>`;

    try {
        const res = await fetch(`/ai-report?symbol=${currentAnalysisData.symbol}`);
        const result = await res.json();
        if (!res.ok) throw new Error(result.error || 'AI Report failed');

        currentAnalysisData.ai_report = result.ai_report;
        reportBody.innerHTML = renderMarkdown(result.ai_report);
    } catch (err) {
        reportBody.innerHTML = `<span class="text-rose-400"><i class="fa-solid fa-circle-exclamation mr-1.5"></i>Error: ${err.message}</span>`;
    } finally {
        btn.disabled = false;
        spinner.classList.remove('fa-spin');
    }
}

// Simple Markdown Renderer
function renderMarkdown(md) {
    if (!md) return '';
    // Strip trailing/leading spaces
    let html = md.trim();
    // Headers
    html = html.replace(/^### (.*$)/gim, '<h5 class="text-sm font-bold text-white mt-4 mb-1 border-b border-slate-800 pb-1">$1</h5>');
    html = html.replace(/^## (.*$)/gim, '<h4 class="text-base font-bold text-emerald-400 mt-5 mb-2">$1</h4>');
    html = html.replace(/^# (.*$)/gim, '<h3 class="text-lg font-bold text-white mt-6 mb-2 border-b border-slate-800 pb-1.5">$1</h3>');
    // Bold
    html = html.replace(/\*\*(.*?)\*\*/gim, '<strong class="text-white font-bold">$1</strong>');
    // Bullet points
    html = html.replace(/^\s*-\s+(.*$)/gim, '<li class="ml-4 list-disc text-slate-300 mt-1 font-sans">$1</li>');
    // Line breaks
    html = html.replace(/\n/gim, '<br>');
    return html;
}

// Chart.js renderers
function renderCharts() {
    if (!currentAnalysisData) return;

    // 1. Render Expiry Chart
    renderExpiryChart();

    // 2. Render Strike Distribution Chart
    renderStrikeDistributionChart();
}

function renderExpiryChart() {
    const canvas = document.getElementById('expiry-chart');
    if (!canvas) return;

    if (expiryChartRef) expiryChartRef.destroy();

    const expiries = currentAnalysisData.expiries || [];
    if (expiries.length === 0) {
        // Mock fallback if empty
        return;
    }

    expiryChartRef = new Chart(canvas, {
        type: 'bar',
        data: {
            labels: expiries.map(e => e.expiry || 'Total'),
            datasets: [
                {
                    label: 'Call OI (Lots)',
                    data: expiries.map(e => e.total_ce_oi),
                    backgroundColor: 'rgba(16, 185, 129, 0.75)',
                    borderColor: 'rgb(16, 185, 129)',
                    borderWidth: 1.5,
                    borderRadius: 6
                },
                {
                    label: 'Put OI (Lots)',
                    data: expiries.map(e => e.total_pe_oi),
                    backgroundColor: 'rgba(244, 63, 94, 0.75)',
                    borderColor: 'rgb(244, 63, 94)',
                    borderWidth: 1.5,
                    borderRadius: 6
                }
            ]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            scales: {
                x: {
                    ticks: { color: '#94a3b8', font: { size: 10, family: 'monospace' } },
                    grid: { color: '#1e293b' }
                },
                y: {
                    ticks: { color: '#94a3b8', font: { size: 10, family: 'monospace' } },
                    grid: { color: '#1e293b' }
                }
            },
            plugins: {
                legend: {
                    labels: { color: '#e2e8f0', font: { size: 11, weight: 'bold' } }
                }
            }
        }
    });
}

function renderStrikeDistributionChart() {
    const canvas = document.getElementById('strike-distribution-chart');
    if (!canvas) return;

    if (strikeChartRef) strikeChartRef.destroy();

    const records = currentAnalysisData.records || [];
    if (records.length === 0) return;

    // Filter to selected expiry to make chart legible
    const select = document.getElementById('matrix-expiry-select');
    const expiry = select ? select.value : (records[0].expiry || 'Current Expiry');
    const filtered = records.filter(r => (r.expiry || 'Current Expiry') === expiry);

    // Group by strike and calculate total OI
    const grouped = {};
    filtered.forEach(r => {
        if (!grouped[r.strike_price]) {
            grouped[r.strike_price] = { strike: r.strike_price, CE: 0, PE: 0, total: 0 };
        }
        grouped[r.strike_price][r.option_type] = r.oi;
        grouped[r.strike_price].total += r.oi;
    });

    // Take top 10 strikes by total OI
    const topStrikes = Object.values(grouped)
        .sort((a, b) => b.total - a.total)
        .slice(0, 10)
        .sort((a, b) => a.strike - b.strike); // sort strikes ascending for plot

    strikeChartRef = new Chart(canvas, {
        type: 'bar',
        data: {
            labels: topStrikes.map(s => s.strike.toLocaleString()),
            datasets: [
                {
                    label: 'Call OI',
                    data: topStrikes.map(s => s.CE),
                    backgroundColor: 'rgba(16, 185, 129, 0.75)',
                    borderColor: 'rgb(16, 185, 129)',
                    borderWidth: 1.5,
                    borderRadius: 4
                },
                {
                    label: 'Put OI',
                    data: topStrikes.map(s => s.PE),
                    backgroundColor: 'rgba(244, 63, 94, 0.75)',
                    borderColor: 'rgb(244, 63, 94)',
                    borderWidth: 1.5,
                    borderRadius: 4
                }
            ]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            scales: {
                x: {
                    ticks: { color: '#94a3b8', font: { size: 10, family: 'monospace' } },
                    grid: { color: '#1e293b' }
                },
                y: {
                    ticks: { color: '#94a3b8', font: { size: 10, family: 'monospace' } },
                    grid: { color: '#1e293b' }
                }
            },
            plugins: {
                legend: {
                    labels: { color: '#e2e8f0', font: { size: 11, weight: 'bold' } }
                }
            }
        }
    });
}

// Load Demo Data
function quickDemo() {
    currentAnalysisData = {
        symbol: 'NIFTY (DEMO)',
        cached: false,
        spot_price: 22045.20,
        metrics: {
            pcr: 1.42,
            max_pain: 22000,
            iv_skew: -1.2,
            bias: 'Bullish',
            bias_reason: 'PCR of 1.42 indicates strong put-writing support built up at 22000, with put sellers actively absorbing downside volatility.',
            total_ce_oi: 8200000,
            total_pe_oi: 11640000,
            top_ce_strikes: [22200, 22500, 22800],
            top_pe_strikes: [22000, 21800, 21500]
        },
        expiries: [
            { expiry: '18-Jun-2026', total_ce_oi: 4200000, total_pe_oi: 5600000, pcr: 1.33, max_pain: 22000, iv_skew: -0.8 },
            { expiry: '25-Jun-2026', total_ce_oi: 2500000, total_pe_oi: 3800000, pcr: 1.52, max_pain: 22000, iv_skew: -1.5 },
            { expiry: '02-Jul-2026', total_ce_oi: 1500000, total_pe_oi: 2240000, pcr: 1.49, max_pain: 22100, iv_skew: -1.0 }
        ],
        signals: [
            { strike_price: 22200, option_type: 'CE', expiry: '18-Jun-2026', action: 'WATCH', rationale: 'CE OI build-up (+195%) indicates resistance building up at 22200.', confidence: 'MEDIUM', source: 'fallback' },
            { strike_price: 22000, option_type: 'PE', expiry: '18-Jun-2026', action: 'BUY', rationale: 'Heavy Put writing (+519%) at 22000 strike. Aggressive institutional support established.', confidence: 'HIGH', source: 'fallback' },
            { strike_price: 21800, option_type: 'PE', expiry: '25-Jun-2026', action: 'BUY', rationale: 'Put OI build-up (+179%) showing institutional support moving higher.', confidence: 'MEDIUM', source: 'fallback' },
            { strike_price: 22500, option_type: 'CE', expiry: '18-Jun-2026', action: 'SELL', rationale: 'CE short covering (-32%) suggests call writers are unwinding due to fear of upward breakout.', confidence: 'MEDIUM', source: 'fallback' }
        ],
        records: [
            // CE Records
            { symbol: 'NIFTY', strike_price: 21800, option_type: 'CE', expiry: '18-Jun-2026', oi: 150000, prev_oi: 140000, oi_change_pct: 7.1, volume: 45000, ltp: 295.40, iv: 11.2 },
            { symbol: 'NIFTY', strike_price: 21900, option_type: 'CE', expiry: '18-Jun-2026', oi: 280000, prev_oi: 250000, oi_change_pct: 12.0, volume: 85000, ltp: 210.15, iv: 11.5 },
            { symbol: 'NIFTY', strike_price: 22000, option_type: 'CE', expiry: '18-Jun-2026', oi: 1250000, prev_oi: 1100000, oi_change_pct: 13.6, volume: 380000, ltp: 138.50, iv: 11.8 },
            { symbol: 'NIFTY', strike_price: 22100, option_type: 'CE', expiry: '18-Jun-2026', oi: 950000, prev_oi: 880000, oi_change_pct: 7.9, volume: 290000, ltp: 81.20, iv: 12.1 },
            { symbol: 'NIFTY', strike_price: 22200, option_type: 'CE', expiry: '18-Jun-2026', oi: 2200000, prev_oi: 745000, oi_change_pct: 195.3, volume: 620000, ltp: 42.60, iv: 12.5 },
            // PE Records
            { symbol: 'NIFTY', strike_price: 21805, option_type: 'PE', expiry: '18-Jun-2026', oi: 1120000, prev_oi: 401000, oi_change_pct: 179.3, volume: 310000, ltp: 12.10, iv: 13.5 },
            { symbol: 'NIFTY', strike_price: 21900, option_type: 'PE', expiry: '18-Jun-2026', oi: 890000, prev_oi: 720000, oi_change_pct: 23.6, volume: 220000, ltp: 24.80, iv: 13.1 },
            { symbol: 'NIFTY', strike_price: 22000, option_type: 'PE', expiry: '18-Jun-2026', oi: 4500000, prev_oi: 726000, oi_change_pct: 519.8, volume: 1150000, ltp: 55.40, iv: 12.7 },
            { symbol: 'NIFTY', strike_price: 22105, option_type: 'PE', expiry: '18-Jun-2026', oi: 720000, prev_oi: 690050, oi_change_pct: 4.3, volume: 180000, ltp: 102.50, iv: 12.3 },
            { symbol: 'NIFTY', strike_price: 22200, option_type: 'PE', expiry: '18-Jun-2026', oi: 320000, prev_oi: 350000, oi_change_pct: -8.5, volume: 92000, ltp: 165.20, iv: 12.0 }
        ]
    };

    renderResults();
    showStatus('✓ Loaded mock options chain data for demonstration.', 'emerald');
}

// Short cut key to symbol focus
document.addEventListener('keydown', e => {
    if (e.key === '/' && document.activeElement.tagName === 'BODY') {
        e.preventDefault();
        const input = document.getElementById('symbol-input');
        if (input) input.focus();
    }
});

function toggleTheme() {
    const html = document.documentElement;
    html.classList.toggle('dark');
    const icon = document.getElementById('theme-icon');
    if (icon) {
        icon.classList.toggle('fa-moon', html.classList.contains('dark'));
        icon.classList.toggle('fa-sun', !html.classList.contains('dark'));
    }
}

function toggleBeginnerGuide() {
    const content = document.getElementById('beginner-guide-content');
    const chevron = document.getElementById('guide-chevron');
    if (!content || !chevron) return;
    
    const isHidden = content.classList.contains('hidden');
    if (isHidden) {
        content.classList.remove('hidden');
        chevron.classList.add('rotate-180');
    } else {
        content.classList.add('hidden');
        chevron.classList.remove('rotate-180');
    }
}

// ==========================================
// Virtual Strategy Builder & Payoff Engine
// ==========================================

let strategyBasket = [];
let payoffChartRef = null;
let targetSpotPrice = 0;
let isDrawerExpanded = false;

function getLotSize(symbol) {
    if (!symbol) return 50;
    const s = symbol.toUpperCase();
    if (s.includes('BANKNIFTY')) return 15;
    if (s.includes('FINNIFTY')) return 40;
    if (s.includes('NIFTY')) return 50;
    return 50;
}

function addToBasket(strike, type, action, ltp, expiry) {
    const symbol = currentAnalysisData?.symbol || 'NIFTY';
    const lotSize = getLotSize(symbol);
    
    // Unique ID for the leg
    const id = `${strike}_${type}_${action}_${expiry}`;
    
    // Check if duplicate leg exists
    const existing = strategyBasket.find(l => l.id === id);
    if (existing) {
        existing.lots += 1;
        existing.qty = existing.lots * existing.lotSize;
    } else {
        strategyBasket.push({
            id: id,
            strike: strike,
            type: type,
            action: action,
            ltp: ltp,
            expiry: expiry,
            lots: 1,
            lotSize: lotSize,
            qty: lotSize
        });
    }
    
    renderBasket();
    
    // Slide up drawer if hidden
    const drawer = document.getElementById('strategy-drawer');
    if (drawer && strategyBasket.length === 1) {
        drawer.classList.remove('translate-y-full');
        // Expand automatically on first leg
        expandDrawer();
    }
    
    showStatus(`✓ Added ${action} ${strike} ${type} to strategy basket.`, 'emerald');
}

function deleteLeg(id) {
    strategyBasket = strategyBasket.filter(l => l.id !== id);
    renderBasket();
    
    if (strategyBasket.length === 0) {
        clearBasket(null, true);
    }
}

function updateLegLots(id, delta) {
    const leg = strategyBasket.find(l => l.id === id);
    if (!leg) return;
    
    leg.lots = Math.max(1, leg.lots + delta);
    leg.qty = leg.lots * leg.lotSize;
    renderBasket();
}

function toggleLegAction(id) {
    const leg = strategyBasket.find(l => l.id === id);
    if (!leg) return;
    
    leg.action = leg.action === 'BUY' ? 'SELL' : 'BUY';
    const newId = `${leg.strike}_${leg.type}_${leg.action}_${leg.expiry}`;
    
    // Check if new id collides
    const collision = strategyBasket.find(l => l.id === newId && l !== leg);
    if (collision) {
        collision.lots += leg.lots;
        collision.qty = collision.lots * collision.lotSize;
        strategyBasket = strategyBasket.filter(l => l !== leg);
    } else {
        leg.id = newId;
    }
    
    renderBasket();
}

function clearBasket(e, showAnimation = true) {
    if (e) e.stopPropagation();
    strategyBasket = [];
    renderBasket();
    
    collapseDrawer();
    const drawer = document.getElementById('strategy-drawer');
    if (drawer) {
        if (!showAnimation) {
            drawer.classList.add('transition-none');
            drawer.classList.add('translate-y-full');
            setTimeout(() => {
                drawer.classList.remove('transition-none');
            }, 50);
        } else {
            drawer.classList.add('translate-y-full');
        }
    }
}

function toggleDrawer() {
    if (isDrawerExpanded) {
        collapseDrawer();
    } else {
        expandDrawer();
    }
}

function expandDrawer() {
    const body = document.getElementById('drawer-body');
    const chevron = document.getElementById('drawer-chevron');
    if (body) body.classList.remove('hidden');
    if (chevron) {
        chevron.classList.add('rotate-180');
    }
    isDrawerExpanded = true;
    document.body.classList.add('pb-80');
}

function collapseDrawer() {
    const body = document.getElementById('drawer-body');
    const chevron = document.getElementById('drawer-chevron');
    if (body) body.classList.add('hidden');
    if (chevron) {
        chevron.classList.remove('rotate-180');
    }
    isDrawerExpanded = false;
    document.body.classList.remove('pb-80');
}

function calculateStrategyStats() {
    const stats = {
        netPremium: 0,
        maxProfit: 0,
        maxLoss: 0,
        rrRatio: 'N/A',
        breakevens: []
    };
    
    if (strategyBasket.length === 0) return stats;
    
    // 1. Net Premium
    stats.netPremium = strategyBasket.reduce((sum, leg) => {
        const premium = (leg.action === 'SELL' ? 1 : -1) * leg.ltp * leg.qty;
        return sum + premium;
    }, 0);
    
    // 2. Max Profit / Max Loss
    const strikes = strategyBasket.map(l => l.strike);
    const spot = currentAnalysisData?.spot_price || strikes[0] || 22000;
    const highestStrike = Math.max(...strikes, spot);
    const lowestStrike = Math.min(...strikes, spot);
    const xHigh = highestStrike + (highestStrike - lowestStrike || 1000) * 1.5;
    const xLow = Math.max(0, lowestStrike - (highestStrike - lowestStrike || 1000) * 0.5);
    
    const criticalPoints = [0, xLow, ...strikes, xHigh];
    const pnlValues = criticalPoints.map(x => getPayoffAt(x));
    
    const slopeHigh = strategyBasket.reduce((sum, leg) => {
        if (leg.type === 'CE') {
            return sum + (leg.action === 'BUY' ? 1 : -1) * leg.qty;
        }
        return sum;
    }, 0);
    
    if (slopeHigh > 0) {
        stats.maxProfit = Infinity;
        stats.maxLoss = Math.abs(Math.min(0, ...pnlValues));
    } else if (slopeHigh < 0) {
        stats.maxProfit = Math.max(0, ...pnlValues);
        stats.maxLoss = Infinity;
    } else {
        stats.maxProfit = Math.max(0, ...pnlValues);
        stats.maxLoss = Math.abs(Math.min(0, ...pnlValues));
    }
    
    // 3. Breakevens
    stats.breakevens = calculateBreakevens();
    
    // 4. Risk Reward Ratio
    if (stats.maxProfit === Infinity) {
        stats.rrRatio = 'Unlimited Profit';
    } else if (stats.maxLoss === Infinity) {
        stats.rrRatio = 'Unlimited Risk';
    } else if (stats.maxLoss === 0) {
        stats.rrRatio = 'Risk-Free';
    } else {
        const ratio = stats.maxProfit / stats.maxLoss;
        stats.rrRatio = `1 : ${ratio.toFixed(2)}`;
    }
    
    return stats;
}

function renderBasket() {
    const tbody = document.getElementById('drawer-legs-tbody');
    const countBadge = document.getElementById('drawer-count-badge');
    if (!tbody || !countBadge) return;
    
    countBadge.textContent = `${strategyBasket.length} Leg${strategyBasket.length === 1 ? '' : 's'}`;
    
    // Update drawer header subtitle with detected strategy name
    const subtitle = document.querySelector('#strategy-drawer p');
    if (subtitle) {
        if (strategyBasket.length > 0) {
            const strat = detectStrategy(strategyBasket);
            subtitle.innerHTML = `<span class="text-emerald-400 font-semibold uppercase tracking-wider">${strat.name}</span> • Click to expand details or adjust legs`;
        } else {
            subtitle.textContent = 'Click to expand details or adjust strategy legs';
        }
    }
    
    if (strategyBasket.length === 0) {
        tbody.innerHTML = `<tr><td colspan="9" class="text-center py-8 text-slate-500 italic">No legs added. Hover over option matrix LTP and click B or S to build a strategy.</td></tr>`;
        
        document.getElementById('drawer-header-net-premium').textContent = '₹0.00';
        document.getElementById('drawer-header-max-profit').textContent = '₹0.00';
        document.getElementById('drawer-header-max-loss').textContent = '₹0.00';
        return;
    }
    
    tbody.innerHTML = strategyBasket.map(leg => {
        const isBuy = leg.action === 'BUY';
        const actionClass = isBuy 
            ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/30 hover:bg-emerald-500/20' 
            : 'bg-rose-500/10 text-rose-400 border border-rose-500/30 hover:bg-rose-500/20';
        
        const typeClass = leg.type === 'CE' ? 'bg-emerald-950 text-emerald-300' : 'bg-rose-950 text-rose-300';
        const legNetPremium = (isBuy ? -1 : 1) * leg.ltp * leg.qty;
        const premiumText = '₹' + Math.abs(legNetPremium).toLocaleString(undefined, {minimumFractionDigits: 2}) + (legNetPremium >= 0 ? ' (Cr)' : ' (Dr)');
        const premiumColor = legNetPremium >= 0 ? 'text-emerald-400' : 'text-rose-400/80';
        
        return `
            <tr class="hover:bg-slate-900/50 border-b border-slate-800/40">
                <td class="py-3">
                    <button onclick="toggleLegAction('${leg.id}')" class="px-2.5 py-0.5 rounded text-[10px] font-bold tracking-wider transition-colors ${actionClass}">
                        ${leg.action}
                    </button>
                </td>
                <td class="py-3">
                    <span class="px-2 py-0.5 rounded text-[10px] font-bold ${typeClass}">
                        ${leg.type}
                    </span>
                </td>
                <td class="py-3 font-bold text-white">${leg.strike.toLocaleString()}</td>
                <td class="py-3 text-slate-400 text-xs">${leg.expiry}</td>
                <td class="py-3 text-slate-400 text-xs">₹${leg.ltp.toFixed(2)}</td>
                <td class="py-3 text-center">
                    <div class="inline-flex items-center gap-1.5 bg-slate-900 border border-slate-800 rounded px-1.5 py-0.5">
                        <button onclick="updateLegLots('${leg.id}', -1)" class="w-4 h-4 flex items-center justify-center bg-slate-800 hover:bg-slate-700 text-slate-300 rounded text-xs transition-colors">-</button>
                        <span class="w-6 text-center font-bold font-mono text-[11px] text-white">${leg.lots}</span>
                        <button onclick="updateLegLots('${leg.id}', 1)" class="w-4 h-4 flex items-center justify-center bg-slate-800 hover:bg-slate-700 text-slate-300 rounded text-xs transition-colors">+</button>
                    </div>
                </td>
                <td class="py-3 text-right text-slate-450 text-xs">${leg.qty.toLocaleString()}</td>
                <td class="py-3 text-right font-bold text-xs ${premiumColor}">${premiumText}</td>
                <td class="py-3 text-center">
                    <button onclick="deleteLeg('${leg.id}')" class="text-slate-500 hover:text-rose-450 transition-colors w-7 h-7 rounded-lg hover:bg-slate-800 flex items-center justify-center mx-auto">
                        <i class="fa-solid fa-trash-can text-xs"></i>
                    </button>
                </td>
            </tr>
        `;
    }).join('');
    
    const stats = calculateStrategyStats();
    
    const premiumText = '₹' + Math.abs(stats.netPremium).toLocaleString(undefined, {minimumFractionDigits: 2}) + (stats.netPremium >= 0 ? ' (Net Credit)' : ' (Net Debit)');
    const headerPremiumEl = document.getElementById('drawer-header-net-premium');
    headerPremiumEl.textContent = premiumText;
    if (stats.netPremium >= 0) {
        headerPremiumEl.className = 'font-bold text-emerald-400';
    } else {
        headerPremiumEl.className = 'font-bold text-white';
    }
    
    document.getElementById('drawer-header-max-profit').textContent = stats.maxProfit === Infinity ? 'Unlimited' : '₹' + stats.maxProfit.toLocaleString(undefined, {minimumFractionDigits: 2});
    document.getElementById('drawer-header-max-loss').textContent = stats.maxLoss === Infinity ? 'Unlimited' : '₹' + stats.maxLoss.toLocaleString(undefined, {minimumFractionDigits: 2});
}

function getPayoffAt(X) {
    return strategyBasket.reduce((sum, leg) => {
        let intrinsic = 0;
        if (leg.type === 'CE') {
            intrinsic = Math.max(0, X - leg.strike);
        } else {
            intrinsic = Math.max(0, leg.strike - X);
        }
        let unitPnl = 0;
        if (leg.action === 'BUY') {
            unitPnl = intrinsic - leg.ltp;
        } else {
            unitPnl = leg.ltp - intrinsic;
        }
        return sum + (unitPnl * leg.qty);
    }, 0);
}

function calculateBreakevens() {
    const strikes = strategyBasket.map(l => l.strike);
    if (strikes.length === 0) return [];
    
    const spot = currentAnalysisData?.spot_price || strikes[0] || 22000;
    const highestStrike = Math.max(...strikes, spot);
    const lowestStrike = Math.min(...strikes, spot);
    const xHigh = highestStrike + (highestStrike - lowestStrike || 1000) * 1.5;
    
    const sortedStrikes = [...new Set(strikes)].sort((a, b) => a - b);
    const points = [0, ...sortedStrikes, xHigh];
    const breakevens = [];
    
    for (let i = 0; i < points.length - 1; i++) {
        const A = points[i];
        const B = points[i + 1];
        const pnlA = getPayoffAt(A);
        const pnlB = getPayoffAt(B);
        
        if (Math.abs(pnlA) < 0.01) {
            breakevens.push(A);
        }
        
        if (pnlA * pnlB < 0) {
            const xBe = A + (B - A) * (0 - pnlA) / (pnlB - pnlA);
            breakevens.push(xBe);
        }
    }
    
    const uniqueBes = [];
    breakevens.sort((a, b) => a - b).forEach(be => {
        if (uniqueBes.length === 0 || be - uniqueBes[uniqueBes.length - 1] > 0.5) {
            uniqueBes.push(be);
        }
    });
    
    return uniqueBes;
}

function detectStrategy(legs) {
    if (!legs || legs.length === 0) return { name: 'No Strategy', badge: 'NONE' };
    
    const calls = legs.filter(l => l.type === 'CE');
    const puts = legs.filter(l => l.type === 'PE');
    
    if (legs.length === 1) {
        const leg = legs[0];
        if (leg.type === 'CE') {
            return leg.action === 'BUY' 
                ? { name: 'Long Call Option', badge: 'L-CALL' }
                : { name: 'Naked Call Writing', badge: 'S-CALL' };
        } else {
            return leg.action === 'BUY' 
                ? { name: 'Long Put Option', badge: 'L-PUT' }
                : { name: 'Naked Put Writing', badge: 'S-PUT' };
        }
    }
    
    if (legs.length === 2) {
        if (calls.length === 2) {
            const buyLeg = calls.find(l => l.action === 'BUY');
            const sellLeg = calls.find(l => l.action === 'SELL');
            if (buyLeg && sellLeg) {
                return buyLeg.strike < sellLeg.strike
                    ? { name: 'Bull Call Spread', badge: 'BULL-CALL' }
                    : { name: 'Bear Call Spread', badge: 'BEAR-CALL' };
            }
        }
        if (puts.length === 2) {
            const buyLeg = puts.find(l => l.action === 'BUY');
            const sellLeg = puts.find(l => l.action === 'SELL');
            if (buyLeg && sellLeg) {
                return buyLeg.strike > sellLeg.strike
                    ? { name: 'Bear Put Spread', badge: 'BEAR-PUT' }
                    : { name: 'Bull Put Spread', badge: 'BULL-PUT' };
            }
        }
        if (calls.length === 1 && puts.length === 1) {
            const cLeg = calls[0];
            const pLeg = puts[0];
            if (cLeg.action === 'BUY' && pLeg.action === 'BUY') {
                return cLeg.strike === pLeg.strike
                    ? { name: 'Long Straddle', badge: 'L-STRADDLE' }
                    : { name: 'Long Strangle', badge: 'L-STRANGLE' };
            }
            if (cLeg.action === 'SELL' && pLeg.action === 'SELL') {
                return cLeg.strike === pLeg.strike
                    ? { name: 'Short Straddle', badge: 'S-STRADDLE' }
                    : { name: 'Short Strangle', badge: 'S-STRANGLE' };
            }
        }
    }
    
    if (legs.length === 4) {
        if (calls.length === 2 && puts.length === 2) {
            const cBuy = calls.find(l => l.action === 'BUY');
            const cSell = calls.find(l => l.action === 'SELL');
            const pBuy = puts.find(l => l.action === 'BUY');
            const pSell = puts.find(l => l.action === 'SELL');
            
            if (cBuy && cSell && pBuy && pSell) {
                if (pBuy.strike < pSell.strike && pSell.strike <= cSell.strike && cSell.strike < cBuy.strike) {
                    return pSell.strike === cSell.strike
                        ? { name: 'Iron Butterfly Strategy', badge: 'IRON-BUTT' }
                        : { name: 'Iron Condor Strategy', badge: 'IRON-COND' };
                }
            }
        }
    }
    
    return { name: 'Custom Options Strategy', badge: 'CUSTOM' };
}

function analyzeStrategyPayoff(e) {
    if (e) e.stopPropagation();
    if (strategyBasket.length === 0) return;
    
    const modal = document.getElementById('payoff-modal');
    const content = document.getElementById('payoff-modal-content');
    if (!modal || !content) return;
    
    modal.classList.remove('hidden');
    setTimeout(() => {
        modal.classList.remove('opacity-0');
        content.classList.remove('scale-95');
    }, 10);
    
    const strat = detectStrategy(strategyBasket);
    document.getElementById('payoff-strategy-name').textContent = strat.name;
    const badge = document.getElementById('payoff-strategy-badge');
    badge.textContent = strat.badge;
    
    const spot = currentAnalysisData?.spot_price || strategyBasket[0].strike;
    targetSpotPrice = spot;
    
    const strikes = strategyBasket.map(l => l.strike);
    const minStrike = Math.min(...strikes, spot);
    const maxStrike = Math.max(...strikes, spot);
    const padding = Math.max(spot * 0.05, (maxStrike - minStrike || spot * 0.1) * 0.4);
    
    const minSpot = Math.max(0, minStrike - padding);
    const maxSpot = maxStrike + padding;
    
    const slider = document.getElementById('target-spot-slider');
    slider.min = minSpot.toFixed(0);
    slider.max = maxSpot.toFixed(0);
    slider.value = spot.toFixed(0);
    
    document.getElementById('slider-min-label').textContent = minSpot.toFixed(0);
    document.getElementById('slider-max-label').textContent = maxSpot.toFixed(0);
    document.getElementById('slider-current-spot').textContent = `Spot: ${spot.toFixed(2)}`;
    
    document.getElementById('target-spot-input').value = spot.toFixed(2);
    
    const stats = calculateStrategyStats();
    
    document.getElementById('payoff-max-profit').textContent = stats.maxProfit === Infinity ? 'Unlimited' : '₹' + stats.maxProfit.toLocaleString(undefined, {minimumFractionDigits: 2});
    document.getElementById('payoff-max-loss').textContent = stats.maxLoss === Infinity ? 'Unlimited' : '₹' + stats.maxLoss.toLocaleString(undefined, {minimumFractionDigits: 2});
    
    const breakevensText = stats.breakevens.length > 0 
        ? stats.breakevens.map(be => be.toFixed(2)).join(', ') 
        : 'None';
    document.getElementById('payoff-breakeven').textContent = breakevensText;
    document.getElementById('payoff-breakeven').title = breakevensText;
    
    const premiumText = '₹' + Math.abs(stats.netPremium).toLocaleString(undefined, {minimumFractionDigits: 2}) + (stats.netPremium >= 0 ? ' (Net Credit)' : ' (Net Debit)');
    const premiumEl = document.getElementById('payoff-net-premium');
    premiumEl.textContent = premiumText;
    if (stats.netPremium >= 0) {
        premiumEl.className = 'text-xs font-bold text-emerald-400 font-mono';
    } else {
        premiumEl.className = 'text-xs font-bold text-white font-mono';
    }
    
    document.getElementById('payoff-rr').textContent = stats.rrRatio;
    
    updateTargetPnL();
    renderPayoffChart(minSpot, maxSpot);
}

function renderPayoffChart(minX, maxX) {
    const canvas = document.getElementById('payoff-canvas');
    if (!canvas) return;
    
    if (payoffChartRef) payoffChartRef.destroy();
    
    const labels = [];
    const pnlData = [];
    const step = (maxX - minX) / 100;
    
    for (let i = 0; i <= 100; i++) {
        const x = minX + i * step;
        labels.push(x.toFixed(2));
        pnlData.push(getPayoffAt(x));
    }
    
    const ctx = canvas.getContext('2d');
    payoffChartRef = new Chart(ctx, {
        type: 'line',
        data: {
            labels: labels,
            datasets: [{
                label: 'Profit / Loss (₹)',
                data: pnlData,
                borderColor: 'rgb(56, 189, 248)',
                borderWidth: 2.5,
                pointRadius: 0,
                pointHoverRadius: 6,
                pointBackgroundColor: 'rgb(255, 255, 255)',
                fill: {
                    target: 'origin',
                    above: 'rgba(16, 185, 129, 0.08)',
                    below: 'rgba(244, 63, 94, 0.08)'
                },
                tension: 0
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            interaction: {
                mode: 'index',
                intersect: false,
            },
            scales: {
                x: {
                    title: { display: true, text: 'Spot Price at Expiry', color: '#94a3b8', font: { size: 10, weight: 'bold' } },
                    ticks: { color: '#64748b', font: { size: 9, family: 'monospace' } },
                    grid: { color: 'rgba(30, 41, 59, 0.5)' }
                },
                y: {
                    title: { display: true, text: 'Projected Profit / Loss', color: '#94a3b8', font: { size: 10, weight: 'bold' } },
                    ticks: { 
                        color: '#64748b', 
                        font: { size: 9, family: 'monospace' },
                        callback: function(value) {
                            return (value >= 0 ? '+' : '') + '₹' + value.toLocaleString();
                        }
                    },
                    grid: { color: 'rgba(30, 41, 59, 0.5)' }
                }
            },
            plugins: {
                legend: { display: false },
                tooltip: {
                    callbacks: {
                        label: function(context) {
                            const val = context.raw;
                            return 'P&L: ' + (val >= 0 ? '₹' : '-₹') + Math.abs(val).toLocaleString(undefined, {minimumFractionDigits: 2});
                        },
                        title: function(context) {
                            return 'Spot Price: ' + parseFloat(context[0].label).toLocaleString();
                        }
                    }
                }
            }
        }
    });
}

function closePayoffModal() {
    const modal = document.getElementById('payoff-modal');
    const content = document.getElementById('payoff-modal-content');
    if (!modal || !content) return;
    
    modal.classList.add('opacity-0');
    content.classList.add('scale-95');
    
    setTimeout(() => {
        modal.classList.add('hidden');
    }, 300);
}

function updateTargetSpotFromSlider(val) {
    targetSpotPrice = parseFloat(val);
    document.getElementById('target-spot-input').value = targetSpotPrice.toFixed(2);
    updateTargetPnL();
}

function updateTargetSpotFromInput(val) {
    const parsed = parseFloat(val);
    if (isNaN(parsed) || parsed < 0) return;
    targetSpotPrice = parsed;
    document.getElementById('target-spot-slider').value = targetSpotPrice;
    updateTargetPnL();
}

function updateTargetPnL() {
    if (!strategyBasket || strategyBasket.length === 0) return;
    const spot = currentAnalysisData?.spot_price || 22000;
    const pnl = getPayoffAt(targetSpotPrice);
    
    const pctChange = ((targetSpotPrice - spot) / spot) * 100;
    const pctEl = document.getElementById('target-percentage');
    if (pctEl) {
        pctEl.textContent = (pctChange >= 0 ? '+' : '') + pctChange.toFixed(2) + '%';
        if (pctChange >= 0) {
            pctEl.className = 'text-[9px] font-mono bg-emerald-950/50 border border-emerald-900 px-1.5 py-0.5 rounded-md text-emerald-450 text-emerald-400';
        } else {
            pctEl.className = 'text-[9px] font-mono bg-rose-950/50 border border-rose-900 px-1.5 py-0.5 rounded-md text-rose-450 text-rose-400';
        }
    }
    
    const pnlEl = document.getElementById('target-pnl');
    if (pnlEl) {
        pnlEl.textContent = (pnl >= 0 ? '+₹' : '-₹') + Math.abs(pnl).toLocaleString(undefined, {minimumFractionDigits: 2});
        if (pnl >= 0) {
            pnlEl.className = 'text-base font-black font-mono text-emerald-400';
        } else {
            pnlEl.className = 'text-base font-black font-mono text-rose-400';
        }
    }
    
    const roiEl = document.getElementById('target-roi');
    if (roiEl) {
        const stats = calculateStrategyStats();
        if (stats.netPremium < 0) {
            const capital = Math.abs(stats.netPremium);
            const roi = (pnl / capital) * 100;
            roiEl.textContent = 'ROI: ' + (roi >= 0 ? '+' : '') + roi.toFixed(1) + '%';
        } else {
            let writtenContracts = 0;
            strategyBasket.forEach(l => {
                if (l.action === 'SELL') {
                    writtenContracts += l.lots;
                }
            });
            if (writtenContracts > 0) {
                const capital = writtenContracts * 150000;
                const roi = (pnl / capital) * 100;
                roiEl.textContent = 'ROI: ' + (roi >= 0 ? '+' : '') + roi.toFixed(2) + '% (Est. Margin)';
            } else {
                roiEl.textContent = 'ROI: —';
            }
        }
    }
}

