// IME bridge for noVNC on Android WebView.
// noVNC binds its keyboard to <canvas>, which never summons the soft keyboard
// because Android IMEs only attach to genuinely editable elements. We therefore
// keep a real 1px textarea around, route every tap on the canvas into it, and
// translate its input into RFB key events ourselves.

const KEYMAP = {
  '\n': 'XK_Return',
  '\b': 'XK_BackSpace',
  '\t': 'XK_Tab',
  '\x1b': 'XK_Escape',
};

// Keysyms the bridge needs by name (declared before attachIME runs).
const RFBKEYS = {
  XK_Return: 0xff0d,
  XK_BackSpace: 0xff08,
  XK_Tab: 0xff09,
  XK_Escape: 0xff1b,
  XK_Delete: 0xffff,
};

// Characters whose keysym differs from their codepoint, or where the physical
// key matters to applications (mantis: the box runs a shell, so Enter/Tab/BS
// must be the real keys, not Unicode text).
function keysymFor(ch) {
  const c = ch.codePointAt(0);
  if (c >= 0x20 && c <= 0x7e) return c;      // ASCII printable == Latin-1 keysym
  if (c === 0x0d) return 0xff0d;             // Return
  if (c === 0x0a) return 0xff0d;
  if (c === 0x08) return 0xff08;             // BackSpace
  if (c === 0x09) return 0xff09;             // Tab
  if (c === 0x1b) return 0xff1b;             // Escape
  // NoVNC maps anything above 0xff to a Unicode keysym with the 0x01000000
  // prefix; that is what we send for CJK and other non-Latin input.
  return 0x01000000 + c;
}

export function attachIME(rfb, hostEl, opts = {}) {
  const ta = document.createElement('textarea');
  ta.setAttribute('autocorrect', 'off');
  ta.setAttribute('autocapitalize', 'none');
  ta.setAttribute('autocomplete', 'off');
  ta.setAttribute('spellcheck', 'false');
  ta.setAttribute('aria-hidden', 'true');
  // 1x1, transparent, pinned under the viewport centre: it must be "visible"
  // enough for the IME handshake yet must never steal a touch.
  ta.style.cssText = [
    'position:fixed', 'left:50%', 'top:50%', 'width:1px', 'height:1px',
    'opacity:0', 'border:0', 'padding:0', 'margin:0', 'resize:none',
    'outline:none', 'background:transparent', 'color:transparent',
    'z-index:-1', 'pointer-events:none', 'overflow:hidden',
  ].join(';');
  document.body.appendChild(ta);

  let composing = false;
  let typedSinceFocus = false;

  function send(keysym, code, down) {
    try { rfb.sendKey(keysym, code, down); } catch (e) { /* not connected yet */ }
  }

  function tap(keysym, code) {
    send(keysym, code, true);
    send(keysym, code, false);
  }

  // Primary path: real keystrokes (hardware keyboard, or an IME that emits
  // composition events). Handled here so BackSpace/Enter stay proper keys.
  ta.addEventListener('keydown', (ev) => {
    if (ev.isComposing) return;             // let compositionend handle it
    const name = KEYMAP[ev.key];
    if (name) { ev.preventDefault(); tap(RFBKEYS[name], ev.code || 'Unidentified'); return; }
    if (ev.key && ev.key.length > 1) return; // Shift/Control/arrow: noVNC's own
  });

  // IME path: Android keyboards with a committed composition. `insertText`
  // gives us the final string in one go, which is what a pinyin user expects.
  ta.addEventListener('compositionstart', () => { composing = true; });
  ta.addEventListener('compositionend', (ev) => {
    composing = false;
    emitText(ev.data || '');
  });

  ta.addEventListener('input', (ev) => {
    if (composing) return;
    const t = ev.inputType || '';
    if (t === 'insertText' || t === 'insertCompositionText') {
      emitText(ev.data || '');
    } else if (t === 'deleteContentBackward') {
      tap(RFBKEYS.XK_BackSpace, 'Backspace');
    } else if (t === 'deleteContentForward') {
      tap(RFBKEYS.XK_Delete, 'Delete');
    } else if (t === 'insertLineBreak') {
      tap(RFBKEYS.XK_Return, 'Enter');
    }
    typedSinceFocus = true;
  });

  // Clear the textarea without losing the IME session: reassigning value to
  // '' keeps focus on Android and lets the next keystroke still raise the IME.
  function emitText(s) {
    if (!s) return;
    for (const ch of s) {
      const ks = keysymFor(ch);
      tap(ks, 'Unidentified');
    }
    ta.value = '';
    typedSinceFocus = true;
  }

  function focusIME() {
    try {
      ta.focus({ preventScroll: true });
    } catch (e) {
      ta.focus();
    }
  }

  // Any touch on the desktop surface pulls the IME up. Using touchstart (not
  // click) keeps it inside the user-gesture window Android requires.
  hostEl.addEventListener('touchstart', (ev) => {
    // A two-finger gesture is a scroll/zoom during noVNC's own handling; still
    // fine to focus, it does not disturb the canvas.
    focusIME();
  }, { passive: true, capture: true });

  hostEl.addEventListener('mousedown', focusIME, { capture: true });

  if (opts.autoOpen !== false) {
    // First connect also arms the IME so the very first tap types.
    hostEl.addEventListener('pointerup', focusIME, { capture: true });
  }

  return {
    focus: focusIME,
    blur: () => ta.blur(),
    element: ta,
    isTyping: () => typedSinceFocus,
  };
}

