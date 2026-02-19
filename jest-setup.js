// Polyfill TextEncoder/TextDecoder for jsdom (required by @grafana/ui)
const { TextEncoder, TextDecoder } = require('util');
global.TextEncoder = TextEncoder;
global.TextDecoder = TextDecoder;

// Polyfill IntersectionObserver for jsdom (required by @grafana/ui ScrollContainer)
global.IntersectionObserver = class IntersectionObserver {
  constructor() {}
  observe() {}
  unobserve() {}
  disconnect() {}
};

// Jest setup provided by Grafana scaffolding
import './.config/jest-setup';
