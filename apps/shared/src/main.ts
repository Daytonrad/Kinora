// Polyfills for 2018 TV engines (webOS 4 = Chromium 53, Tizen 4 = Chromium 56):
// only what the app actually uses and those engines lack.
import 'core-js/actual/string/pad-start';
import 'core-js/actual/object/entries';
import 'core-js/actual/object/values';
import 'core-js/actual/object/assign';
import 'core-js/actual/array/includes';
import 'core-js/actual/promise/finally';
import 'core-js/actual/url-search-params';
import 'core-js/actual/weak-map';

import './styles/tokens.gen.css';
import './styles/app.css';

import { state, render, installKeys, initMode, api } from './app';
import { initPlatform } from './platform';
import { applyServerTheme } from './theme';
import './views/auth';
import './views/browse';
import './views/player';
import './views/settings';

// NodeList.forEach (Chromium 51+) and Element.closest are used by the focus engine.
if (window.NodeList && !NodeList.prototype.forEach) (NodeList.prototype as any).forEach = Array.prototype.forEach;

async function boot() {
  await initPlatform();
  initMode();
  installKeys();
  window.addEventListener('hashchange', () => {
    render().then(() => { if (state.api) applyServerTheme(api()); });
  });
  await render();
}

boot();
