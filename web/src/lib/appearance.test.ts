// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { AppearanceStore, roughPlace } from './appearance.svelte';

const html = () => document.documentElement;
const at = (h: number) => () => new Date(2026, 9, 11, h, 0, 0);
const written: string[] = [];
const writer = (c: string) => { written.push(c); document.cookie = c; };

function reset(): void {
  document.cookie = 'vk_appearance=; Max-Age=0; Path=/';
  for (const a of ['data-vibe', 'data-theme', 'data-phase', 'data-time', 'data-accent']) html().removeAttribute(a);
  localStorage.clear();
  written.length = 0;
}

describe('AppearanceStore', () => {
  let s: AppearanceStore;
  beforeEach(() => { reset(); s = new AppearanceStore(writer); });
  afterEach(() => s.dispose());

  it('a first visit is Daylight following the sun: light at noon', () => {
    s.init(at(13));
    expect(html().getAttribute('data-phase')).toBe('noon');
    expect(html().getAttribute('data-time')).toBe('strong');
  });

  it('following the sun is dark at night', () => {
    s.init(at(23));
    expect(s.theme).toBe('dark');
  });

  it('reads the cookie', () => {
    document.cookie = 'vk_appearance=paper.dark.subtle.0; Path=/';
    s.init(at(13));
    expect(s.value).toMatchObject({ vibe: 'paper', theme: 'dark', timeStrength: 'subtle', useLocation: false });
  });

  it('a malformed cookie falls back to the defaults and does not throw', () => {
    document.cookie = 'vk_appearance=neon.%3Cimg%3E; Path=/';
    expect(() => s.init(at(13))).not.toThrow();
    expect(s.value.vibe).toBe('daylight');
    expect(s.value.theme).toBe('sun');
  });

  it('set writes the cookie for this host only', () => {
    s.init(at(13));
    s.set({ vibe: 'studio' });
    const c = written.at(-1)!;
    expect(c).toMatch(/^vk_appearance=studio\.sun\.strong\.0; /);
    expect(c).not.toMatch(/Domain=/i);
  });

  it('choosing Quiet turns time of day off while it was still the default', () => {
    s.init(at(13));
    s.set({ vibe: 'quiet' });
    expect(s.value.timeStrength).toBe('off');
    expect(html().getAttribute('data-time')).toBe('off');
  });

  it('a strength the person chose survives a vibe change', () => {
    s.init(at(13));
    s.set({ timeStrength: 'subtle' });
    s.set({ vibe: 'quiet' });
    expect(s.value.timeStrength).toBe('subtle');
  });

  it('toggleTheme picks the opposite of what is showing', () => {
    s.init(at(13));
    s.toggleTheme();
    expect(s.value.theme).toBe('dark');
    s.toggleTheme();
    expect(s.value.theme).toBe('light');
  });

  it('turning location off forgets the place', () => {
    localStorage.setItem('superwitness.place', JSON.stringify({ latitude: 51.5, longitude: -0.1 }));
    document.cookie = 'vk_appearance=daylight.sun.strong.1; Path=/';
    s.init(at(13));
    expect(s.place).toEqual({ latitude: 51.5, longitude: -0.1 });
    s.set({ useLocation: false });
    expect(s.place).toBeNull();
    expect(localStorage.getItem('superwitness.place')).toBeNull();
  });

  it('a refused location leaves the switch off and does not throw', async () => {
    s.init(at(13));
    const geo = { getCurrentPosition: (_ok: unknown, no: (e: unknown) => void) => no(new Error('denied')) } as unknown as Geolocation;
    await expect(s.useLocation(geo)).resolves.toBe('denied');
    expect(s.value.useLocation).toBe(false);
  });

  it('stopUsingLocation turns it off and forgets the place', () => {
    localStorage.setItem('superwitness.place', JSON.stringify({ latitude: 51.5, longitude: -0.1 }));
    document.cookie = 'vk_appearance=daylight.sun.strong.1; Path=/';
    s.init(at(13));
    s.stopUsingLocation();
    expect(s.value.useLocation).toBe(false);
    expect(s.place).toBeNull();
    expect(localStorage.getItem('superwitness.place')).toBeNull();
    expect(written.at(-1)).toMatch(/^vk_appearance=daylight\.sun\.strong\.0; /);
  });

  it('a request that resolves after the switch was turned off does not turn it back on', async () => {
    s.init(at(13));
    let answer: (p: unknown) => void = () => {};
    const geo = { getCurrentPosition: (ok: (p: unknown) => void) => { answer = ok; } } as unknown as Geolocation;
    const pending = s.useLocation(geo);
    s.stopUsingLocation();
    answer({ coords: { latitude: 51.5072, longitude: -0.1276 } });
    await expect(pending).resolves.toBe('cancelled');
    expect(s.value.useLocation).toBe(false);
    expect(s.place).toBeNull();
    expect(localStorage.getItem('superwitness.place')).toBeNull();
  });

  it('a granted location is stored rough, on this device', async () => {
    s.init(at(13));
    const geo = { getCurrentPosition: (ok: (p: unknown) => void) => ok({ coords: { latitude: 51.5072, longitude: -0.1276 } }) } as unknown as Geolocation;
    await expect(s.useLocation(geo)).resolves.toBe('on');
    expect(JSON.parse(localStorage.getItem('superwitness.place')!)).toEqual({ latitude: 51.5, longitude: -0.1 });
    expect(written.at(-1)).toMatch(/^vk_appearance=daylight\.sun\.strong\.1; /);
  });

  it('forgets the legacy theme key once read', () => {
    localStorage.setItem('superwitness.theme', 'light');
    s.init(at(13));
    expect(localStorage.getItem('superwitness.theme')).toBeNull();
  });

  it('seeds the cookie once from the old light/dark toggle when there is no cookie yet', () => {
    localStorage.setItem('superwitness.theme', 'dark');
    s.init(at(13));
    expect(s.value).toMatchObject({ vibe: 'daylight', theme: 'dark' });
    expect(written.at(-1)).toMatch(/^vk_appearance=daylight\.dark\./);
    expect(localStorage.getItem('superwitness.theme')).toBeNull();
  });

  it('an existing cookie wins over the old toggle', () => {
    document.cookie = 'vk_appearance=paper.light.subtle.0; Path=/';
    localStorage.setItem('superwitness.theme', 'dark');
    s.init(at(13));
    expect(s.value).toMatchObject({ vibe: 'paper', theme: 'light' });
  });

  it('rounds a place to one decimal', () => {
    expect(roughPlace({ latitude: 51.5072, longitude: -0.1276 })).toEqual({ latitude: 51.5, longitude: -0.1 });
  });
});

describe('accent', () => {
  it('defaults to the product colour, writes the choice into the cookie and paints data-accent', () => {
    let cookie = '';
    reset();
    document.documentElement.setAttribute('data-product', 'superwitness');
    const store = new AppearanceStore((c) => { cookie = c; });
    store.init();
    expect(store.accent).toBe('product');
    store.setAccent('violet');
    expect(cookie).toMatch(/superwitness:violet/);
    expect(document.documentElement.dataset.accent).toBe('violet');
    store.setAccent('product');
    expect(cookie).not.toMatch(/superwitness:/);
    expect(document.documentElement.dataset.accent).toBe('product');
    store.dispose();
  });
  it('paints data-vibe and data-theme too, from the same choice', () => {
    reset();
    const el = document.documentElement;
    const store = new AppearanceStore(() => {});
    store.init();
    store.setAccent('jade');
    expect(el.dataset.accent).toBe('jade');
    expect(el.dataset.phase).toBeTruthy();
    store.set({ vibe: 'paper', theme: 'dark' });
    expect([el.dataset.vibe, el.dataset.theme]).toEqual(['paper', 'dark']);
    store.dispose();
  });
});
