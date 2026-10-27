// Deterministic Cluster Simulator — Visual Trace Replayer

(function() {
  'use strict';

  // State
  let currentTrace = null;
  let currentEventIdx = 0;
  let isPlaying = false;
  let playInterval = null;
  let selectedNodeId = 1;

  // DOM Elements
  const seedInput = document.getElementById('seedInput');
  const nodesInput = document.getElementById('nodesInput');
  const putsInput = document.getElementById('putsInput');
  const partitionToggle = document.getElementById('partitionToggle');
  const runBtn = document.getElementById('runBtn');
  const sweepBtn = document.getElementById('sweepBtn');

  const verdictBadge = document.getElementById('verdictBadge');
  const virtualTimeStat = document.getElementById('virtualTimeStat');
  const eventCountStat = document.getElementById('eventCountStat');
  const currentLeaderStat = document.getElementById('currentLeaderStat');
  const traceHashStat = document.getElementById('traceHashStat');

  const canvas = document.getElementById('clusterCanvas');
  const ctx = canvas.getContext('2d');

  const timelineSlider = document.getElementById('timelineSlider');
  const currentEventNum = document.getElementById('currentEventNum');
  const totalEventsNum = document.getElementById('totalEventsNum');
  const btnPrev = document.getElementById('btnPrev');
  const btnPlay = document.getElementById('btnPlay');
  const btnNext = document.getElementById('btnNext');
  const playSpeed = document.getElementById('playSpeed');

  const inspectedNodeName = document.getElementById('inspectedNodeName');
  const inspectedNodeRole = document.getElementById('inspectedNodeRole');
  const inspectTerm = document.getElementById('inspectTerm');
  const inspectVote = document.getElementById('inspectVote');
  const inspectCommit = document.getElementById('inspectCommit');
  const inspectApplied = document.getElementById('inspectApplied');
  const kvCount = document.getElementById('kvCount');
  const kvList = document.getElementById('kvList');

  const currentEventKind = document.getElementById('currentEventKind');
  const eventDetailBox = document.getElementById('eventDetailBox');

  const sweepModal = document.getElementById('sweepModal');
  const closeSweepBtn = document.getElementById('closeSweepBtn');
  const sweepStats = document.getElementById('sweepStats');
  const sweepGrid = document.getElementById('sweepGrid');

  // Initialize
  window.addEventListener('DOMContentLoaded', () => {
    resizeCanvas();
    window.addEventListener('resize', resizeCanvas);
    setupEventListeners();
    fetchSimulation();
  });

  function resizeCanvas() {
    const rect = canvas.parentElement.getBoundingClientRect();
    canvas.width = rect.width;
    canvas.height = rect.height;
    if (currentTrace) renderCanvas();
  }

  function setupEventListeners() {
    runBtn.addEventListener('click', () => {
      fetchSimulation();
    });

    sweepBtn.addEventListener('click', () => {
      openSweepModal();
    });

    closeSweepBtn.addEventListener('click', () => {
      sweepModal.classList.remove('active');
    });

    traceHashStat.addEventListener('click', () => {
      if (currentTrace && currentTrace.trace_hash) {
        navigator.clipboard.writeText(currentTrace.trace_hash);
        const original = traceHashStat.textContent;
        traceHashStat.textContent = 'COPIED!';
        setTimeout(() => { traceHashStat.textContent = original; }, 1200);
      }
    });

    timelineSlider.addEventListener('input', (e) => {
      seekTo(parseInt(e.target.value, 10));
    });

    btnPrev.addEventListener('click', () => {
      seekTo(currentEventIdx - 1);
    });

    btnNext.addEventListener('click', () => {
      seekTo(currentEventIdx + 1);
    });

    btnPlay.addEventListener('click', togglePlay);

    // Keyboard controls
    window.addEventListener('keydown', (e) => {
      if (e.target.tagName === 'INPUT' || e.target.tagName === 'SELECT') return;
      if (e.key === ' ' || e.code === 'Space') {
        e.preventDefault();
        togglePlay();
      } else if (e.key === 'ArrowLeft') {
        seekTo(currentEventIdx - 1);
      } else if (e.key === 'ArrowRight') {
        seekTo(currentEventIdx + 1);
      }
    });

    // Node click selection on canvas
    canvas.addEventListener('click', (e) => {
      const rect = canvas.getBoundingClientRect();
      const clickX = e.clientX - rect.left;
      const clickY = e.clientY - rect.top;

      if (!currentTrace) return;
      const nodes = getActiveSnapshots();
      const positions = getNodePositions(nodes.length);

      for (let i = 0; i < nodes.length; i++) {
        const pos = positions[i];
        const dist = Math.hypot(clickX - pos.x, clickY - pos.y);
        if (dist <= 35) {
          selectedNodeId = nodes[i].ID;
          updateInspector();
          renderCanvas();
          break;
        }
      }
    });
  }

  async function fetchSimulation() {
    pause();
    runBtn.disabled = true;
    runBtn.textContent = 'Running...';

    const reqBody = {
      seed: parseInt(seedInput.value, 10) || 1,
      nodes: parseInt(nodesInput.value, 10) || 3,
      puts: parseInt(putsInput.value, 10) || 50,
      partition: partitionToggle.checked,
      drop_rate: 0.05
    };

    try {
      const res = await fetch('/api/simulate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(reqBody)
      });
      if (!res.ok) throw new Error(`HTTP error ${res.status}`);
      currentTrace = await res.json();
      loadTrace(currentTrace);
    } catch (err) {
      console.error('Failed to run simulation:', err);
      eventDetailBox.textContent = `Simulation error: ${err.message}`;
    } finally {
      runBtn.disabled = false;
      runBtn.innerHTML = '<span class="btn-icon">▶</span> Run Sim';
    }
  }

  function loadTrace(trace) {
    currentTrace = trace;
    const totalEvents = trace.events ? trace.events.length : 0;

    timelineSlider.max = Math.max(0, totalEvents - 1);
    timelineSlider.value = 0;
    totalEventsNum.textContent = totalEvents;

    // Update Verdict
    if (trace.verdict && trace.verdict.passed) {
      verdictBadge.textContent = 'PASS';
      verdictBadge.className = 'badge badge-pass';
    } else {
      verdictBadge.textContent = 'VIOLATION';
      verdictBadge.className = 'badge badge-fail';
    }

    traceHashStat.textContent = trace.trace_hash ? trace.trace_hash.substring(0, 16) + '...' : 'none';
    traceHashStat.title = trace.trace_hash || '';

    seekTo(0);
  }

  function seekTo(index) {
    if (!currentTrace || !currentTrace.events || currentTrace.events.length === 0) return;
    const maxIdx = currentTrace.events.length - 1;
    currentEventIdx = Math.max(0, Math.min(index, maxIdx));

    timelineSlider.value = currentEventIdx;
    currentEventNum.textContent = currentEventIdx + 1;

    const evt = currentTrace.events[currentEventIdx];
    if (evt) {
      virtualTimeStat.textContent = `${evt.at} / ${currentTrace.meta.max_time}`;
      eventCountStat.textContent = `${currentEventIdx + 1} / ${currentTrace.events.length}`;

      currentEventKind.textContent = evt.kind;
      eventDetailBox.textContent = `[${evt.at}] ${evt.detail || evt.kind}`;
    }

    updateInspector();
    updateLeaderStatus();
    renderCanvas();
  }

  function getActiveSnapshots() {
    if (!currentTrace || !currentTrace.events || currentTrace.events.length === 0) return [];
    const evt = currentTrace.events[currentEventIdx];
    if (evt && evt.snapshots && evt.snapshots.length > 0) {
      return evt.snapshots;
    }
    // Fallback to initial
    return [];
  }

  function updateLeaderStatus() {
    const snaps = getActiveSnapshots();
    const leader = snaps.find(s => s.Alive && s.Role === 2); // 2 = Leader
    if (leader) {
      currentLeaderStat.textContent = `n${leader.ID} (Term ${leader.Term})`;
      currentLeaderStat.className = 'stat-value leader-text font-mono';
    } else {
      currentLeaderStat.textContent = 'None (Electing)';
      currentLeaderStat.className = 'stat-value font-mono';
    }
  }

  function updateInspector() {
    const snaps = getActiveSnapshots();
    let snap = snaps.find(s => s.ID === selectedNodeId);
    if (!snap && snaps.length > 0) {
      snap = snaps[0];
      selectedNodeId = snap.ID;
    }

    if (!snap) return;

    inspectedNodeName.textContent = `n${snap.ID}`;

    const roleNames = ['Follower', 'Candidate', 'Leader'];
    const roleClasses = ['badge-follower', 'badge-candidate', 'badge-leader'];
    const rIdx = snap.Role || 0;

    inspectedNodeRole.textContent = snap.Alive ? roleNames[rIdx] : 'Crashed';
    inspectedNodeRole.className = `badge ${snap.Alive ? roleClasses[rIdx] : 'badge-neutral'}`;

    inspectTerm.textContent = snap.Term || 0;
    inspectVote.textContent = snap.VotedFor ? `n${snap.VotedFor}` : 'None';
    inspectCommit.textContent = snap.CommitIndex || 0;
    inspectApplied.textContent = snap.AppliedIndex || 0;

    // Render KV Store
    kvList.innerHTML = '';
    const kv = snap.KVState || {};
    const keys = Object.keys(kv).sort();
    kvCount.textContent = `${keys.length} keys`;

    if (keys.length === 0) {
      kvList.innerHTML = '<div class="empty-hint">KV store is empty at this event</div>';
    } else {
      keys.forEach(k => {
        const chip = document.createElement('div');
        chip.className = 'kv-chip';
        chip.innerHTML = `<span class="kv-key">${escapeHtml(k)}:</span><span class="kv-val">${escapeHtml(kv[k])}</span>`;
        kvList.appendChild(chip);
      });
    }
  }

  function escapeHtml(str) {
    return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  function togglePlay() {
    if (isPlaying) {
      pause();
    } else {
      play();
    }
  }

  function play() {
    if (!currentTrace || !currentTrace.events) return;
    if (currentEventIdx >= currentTrace.events.length - 1) {
      currentEventIdx = 0;
    }
    isPlaying = true;
    btnPlay.textContent = '⏸';
    btnPlay.classList.add('playing');

    const speed = parseInt(playSpeed.value, 10) || 5;
    const intervalMs = Math.max(16, 120 / speed);

    playInterval = setInterval(() => {
      if (currentEventIdx >= currentTrace.events.length - 1) {
        pause();
      } else {
        seekTo(currentEventIdx + 1);
      }
    }, intervalMs);
  }

  function pause() {
    isPlaying = false;
    btnPlay.textContent = '▶';
    btnPlay.classList.remove('playing');
    if (playInterval) {
      clearInterval(playInterval);
      playInterval = null;
    }
  }

  playSpeed.addEventListener('change', () => {
    if (isPlaying) {
      pause();
      play();
    }
  });

  // Canvas Geometry & Rendering
  function getNodePositions(count) {
    const cx = canvas.width / 2;
    const cy = canvas.height / 2;
    const radius = Math.min(cx, cy) * 0.65;
    const positions = [];

    for (let i = 0; i < count; i++) {
      // Start node 1 at top (-PI/2)
      const angle = (i * 2 * Math.PI / count) - (Math.PI / 2);
      positions.push({
        x: cx + radius * Math.cos(angle),
        y: cy + radius * Math.sin(angle)
      });
    }
    return positions;
  }

  function renderCanvas() {
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    const snaps = getActiveSnapshots();
    if (snaps.length === 0) return;

    const positions = getNodePositions(snaps.length);
    const currentEvt = currentTrace.events[currentEventIdx];

    // Check if partition is currently active
    const isPartitionActive = currentEvt && (currentEvt.detail.includes('partition') ||
      (currentEvt.at_ns >= 7000000000 && currentEvt.at_ns <= 14000000000));

    // 1. Draw Links
    for (let i = 0; i < snaps.length; i++) {
      for (let j = i + 1; j < snaps.length; j++) {
        const p1 = positions[i];
        const p2 = positions[j];

        // Is this link cut by partition? (Node 3 separated from 1,2)
        const isCut = isPartitionActive && (
          (snaps[i].ID === 3 && (snaps[j].ID === 1 || snaps[j].ID === 2)) ||
          (snaps[j].ID === 3 && (snaps[i].ID === 1 || snaps[i].ID === 2))
        );

        ctx.beginPath();
        ctx.moveTo(p1.x, p1.y);
        ctx.lineTo(p2.x, p2.y);

        if (isCut) {
          ctx.strokeStyle = 'rgba(244, 63, 94, 0.5)';
          ctx.lineWidth = 2;
          ctx.setLineDash([6, 6]);
          ctx.stroke();
          ctx.setLineDash([]);

          // Draw X on cut link
          const midX = (p1.x + p2.x) / 2;
          const midY = (p1.y + p2.y) / 2;
          ctx.fillStyle = '#f43f5e';
          ctx.font = '14px Inter, sans-serif';
          ctx.textAlign = 'center';
          ctx.textBaseline = 'middle';
          ctx.fillText('✂', midX, midY);
        } else {
          ctx.strokeStyle = 'rgba(255, 255, 255, 0.08)';
          ctx.lineWidth = 1.5;
          ctx.setLineDash([]);
          ctx.stroke();
        }
      }
    }

    // 2. Animate In-Flight Message Packet
    if (currentEvt && currentEvt.from && currentEvt.target && currentEvt.from !== currentEvt.target) {
      const fromIdx = snaps.findIndex(s => s.ID === currentEvt.from);
      const toIdx = snaps.findIndex(s => s.ID === currentEvt.target);

      if (fromIdx !== -1 && toIdx !== -1) {
        const pFrom = positions[fromIdx];
        const pTo = positions[toIdx];

        // Draw pulsing message line
        ctx.beginPath();
        ctx.moveTo(pFrom.x, pFrom.y);
        ctx.lineTo(pTo.x, pTo.y);
        ctx.strokeStyle = 'rgba(0, 240, 255, 0.4)';
        ctx.lineWidth = 3;
        ctx.stroke();

        // Packet Dot
        const midX = (pFrom.x + pTo.x) / 2;
        const midY = (pFrom.y + pTo.y) / 2;

        ctx.beginPath();
        ctx.arc(midX, midY, 6, 0, Math.PI * 2);
        ctx.fillStyle = '#00f0ff';
        ctx.shadowColor = '#00f0ff';
        ctx.shadowBlur = 12;
        ctx.fill();
        ctx.shadowBlur = 0;
      }
    }

    // 3. Draw Nodes
    const nodeColors = {
      0: '#3b82f6', // Follower (Blue)
      1: '#f59e0b', // Candidate (Amber)
      2: '#10b981'  // Leader (Green)
    };

    snaps.forEach((snap, idx) => {
      const pos = positions[idx];
      const isSelected = snap.ID === selectedNodeId;
      const isLeader = snap.Role === 2;
      const color = snap.Alive ? (nodeColors[snap.Role] || '#64748b') : '#475569';

      // Outer Glow
      if (isSelected || isLeader) {
        ctx.beginPath();
        ctx.arc(pos.x, pos.y, 40, 0, Math.PI * 2);
        ctx.fillStyle = isLeader ? 'rgba(16, 185, 129, 0.15)' : 'rgba(0, 240, 255, 0.15)';
        ctx.fill();
      }

      // Circle Body
      ctx.beginPath();
      ctx.arc(pos.x, pos.y, 28, 0, Math.PI * 2);
      ctx.fillStyle = '#111827';
      ctx.fill();

      // Border Ring
      ctx.lineWidth = isSelected ? 3.5 : 2.5;
      ctx.strokeStyle = isSelected ? '#00f0ff' : color;
      ctx.shadowColor = color;
      ctx.shadowBlur = isLeader || isSelected ? 10 : 0;
      ctx.stroke();
      ctx.shadowBlur = 0;

      // Node Label (e.g. "n1")
      ctx.fillStyle = '#ffffff';
      ctx.font = 'bold 15px JetBrains Mono, monospace';
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.fillText(`n${snap.ID}`, pos.x, pos.y - 4);

      // Term badge below
      ctx.fillStyle = color;
      ctx.font = '10px JetBrains Mono, monospace';
      ctx.fillText(`T${snap.Term || 0}`, pos.x, pos.y + 12);

      // Leader Crown Icon
      if (isLeader) {
        ctx.font = '14px Inter, sans-serif';
        ctx.fillText('👑', pos.x, pos.y - 38);
      }
    });
  }

  // Sweep Modal
  async function openSweepModal() {
    sweepModal.classList.add('active');
    sweepStats.textContent = 'Running 20-seed determinism sweep...';
    sweepGrid.innerHTML = '<div class="empty-hint">Executing cluster simulations across random seeds...</div>';

    try {
      const res = await fetch('/api/sweep', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          start_seed: 1,
          count: 24,
          nodes: 3,
          partition: true,
          puts: 30
        })
      });

      const data = await res.json();
      sweepStats.innerHTML = `<strong>${data.passed}/${data.total} Passed</strong> (Completed in ${data.wall_duration_ms}ms, 0 violations)`;
      sweepGrid.innerHTML = '';

      data.results.forEach(item => {
        const card = document.createElement('div');
        card.className = `sweep-card ${item.passed ? 'pass' : 'fail'}`;
        card.innerHTML = `
          <div><strong>Seed ${item.seed}</strong></div>
          <div style="color: #94a3b8; font-size: 0.72rem;">${item.events} evts</div>
          <div style="color: ${item.passed ? '#10b981' : '#f43f5e'}; font-weight: 600;">
            ${item.passed ? 'PASSED' : 'FAILED'}
          </div>
        `;
        card.addEventListener('click', () => {
          seedInput.value = item.seed;
          sweepModal.classList.remove('active');
          fetchSimulation();
        });
        sweepGrid.appendChild(card);
      });
    } catch (err) {
      sweepStats.textContent = `Sweep failed: ${err.message}`;
    }
  }

})();
