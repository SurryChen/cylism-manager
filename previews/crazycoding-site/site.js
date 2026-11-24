// 与腾讯云备案记录中的网站备案号一致。
const siteConfig = { icpNumber: '粤ICP备2026170969号-1' };

document.getElementById('year').textContent = String(new Date().getFullYear());
if (siteConfig.icpNumber) {
  document.getElementById('icp-link').textContent = siteConfig.icpNumber;
}

const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)');
const finePointer = window.matchMedia('(hover: hover) and (pointer: fine)');
const art = document.querySelector('.hero-art');
const motionToggle = document.querySelector('.motion-toggle');
let paused = false;
const hero = document.querySelector('.hero');
let pointerFrame = 0;
let lastFrameTime = 0;
let pointerActive = false;
const pointer = { x: 0, y: 0, targetX: 0, targetY: 0, vx: 0, vy: 0 };

function paintPointer() {
  art.style.setProperty('--tilt-x', `${-pointer.y * 7}deg`);
  art.style.setProperty('--tilt-y', `${pointer.x * 9}deg`);
  art.style.setProperty('--depth-x', `${pointer.x * 24}px`);
  art.style.setProperty('--depth-y', `${pointer.y * 18}px`);
  art.style.setProperty('--light-x', `${50 + pointer.x * 42}%`);
  art.style.setProperty('--light-y', `${50 + pointer.y * 42}%`);
}

function animatePointer(now) {
  // Normalize spring integration to keep the same feel on 60/120Hz displays.
  const delta = Math.min((now - (lastFrameTime || now - 16.67)) / 16.67, 2);
  lastFrameTime = now;
  for (const axis of ['x', 'y']) {
    const velocity = axis === 'x' ? 'vx' : 'vy';
    const target = axis === 'x' ? 'targetX' : 'targetY';
    pointer[velocity] += (pointer[target] - pointer[axis]) * .065 * delta;
    pointer[velocity] *= Math.pow(.72, delta);
    pointer[axis] += pointer[velocity] * delta;
  }
  paintPointer();
  const moving = Math.abs(pointer.x - pointer.targetX) + Math.abs(pointer.y - pointer.targetY)
    + Math.abs(pointer.vx) + Math.abs(pointer.vy) > .001;
  if (moving) pointerFrame = requestAnimationFrame(animatePointer);
  else {
    pointerFrame = 0;
    lastFrameTime = 0;
    if (!pointerActive) resetTilt();
  }
}

function startPointer() {
  if (!pointerFrame) pointerFrame = requestAnimationFrame(animatePointer);
}

function resetTilt() {
  cancelAnimationFrame(pointerFrame);
  pointerFrame = 0;
  lastFrameTime = 0;
  pointerActive = false;
  Object.keys(pointer).forEach((key) => { pointer[key] = 0; });
  art.classList.remove('is-tracking');
  for (const property of ['--tilt-x', '--tilt-y', '--depth-x', '--depth-y', '--light-x', '--light-y']) {
    art.style.removeProperty(property);
  }
  document.querySelectorAll('.explore-row').forEach((row) => row.style.removeProperty('--spot-x'));
}

motionToggle.addEventListener('click', () => {
  paused = !paused;
  document.documentElement.classList.toggle('motion-paused', paused);
  motionToggle.setAttribute('aria-pressed', String(paused));
  motionToggle.setAttribute('aria-label', paused ? '播放动画' : '暂停动画');
  motionToggle.title = paused ? '播放动画' : '暂停动画';
  resetTilt();
});

hero.addEventListener('pointermove', (event) => {
  if (paused || reducedMotion.matches || !finePointer.matches || event.pointerType === 'touch') return;
  const bounds = hero.getBoundingClientRect();
  pointer.targetX = Math.max(-1, Math.min(1, (event.clientX - bounds.left) / bounds.width * 2 - 1));
  pointer.targetY = Math.max(-1, Math.min(1, (event.clientY - bounds.top) / bounds.height * 2 - 1));
  pointerActive = true;
  art.classList.add('is-tracking');
  startPointer();
});
hero.addEventListener('pointerleave', () => {
  pointerActive = false;
  pointer.targetX = 0;
  pointer.targetY = 0;
  art.classList.remove('is-tracking');
  if (!paused && !reducedMotion.matches) startPointer();
});
hero.addEventListener('pointercancel', resetTilt);

// The highlight follows the real pointer; native cursor and text stay unchanged.
document.querySelectorAll('.explore-row').forEach((row) => {
  row.addEventListener('pointermove', (event) => {
    if (paused || reducedMotion.matches || !finePointer.matches || event.pointerType === 'touch') return;
    row.style.setProperty('--spot-x', `${event.clientX - row.getBoundingClientRect().left}px`);
  });
  row.addEventListener('pointerleave', () => row.style.removeProperty('--spot-x'));
});
reducedMotion.addEventListener('change', resetTilt);
finePointer.addEventListener('change', resetTilt);

// Content remains visible without JavaScript or IntersectionObserver support.
if ('IntersectionObserver' in window) {
  const revealObserver = new IntersectionObserver((entries) => {
    entries.forEach((entry) => {
      if (!entry.isIntersecting) return;
      entry.target.classList.add('is-visible');
      revealObserver.unobserve(entry.target);
    });
  }, { threshold: .12 });
  document.querySelectorAll('[data-reveal]').forEach((element) => revealObserver.observe(element));
  document.documentElement.classList.add('motion-ready');

  const artObserver = new IntersectionObserver(([entry]) => {
    art.classList.toggle('is-offscreen', !entry.isIntersecting);
    if (!entry.isIntersecting) resetTilt();
  });
  artObserver.observe(art);
}

document.addEventListener('visibilitychange', () => {
  document.documentElement.classList.toggle('page-hidden', document.hidden);
  if (document.hidden) resetTilt();
});
