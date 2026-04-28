// This must load before inputview.js. The vendor code registers touch/pointer
// listeners without {passive:false} but calls preventDefault() from them.
// Patch addEventListener first so those listeners are registered as non-passive.
(function() {
  var orig = EventTarget.prototype.addEventListener;
  EventTarget.prototype.addEventListener = function(type, fn, opts) {
    if (type === 'touchstart' || type === 'touchmove' || type === 'touchend' ||
        type === 'pointerdown' || type === 'pointermove') {
      if (typeof opts === 'object' && opts !== null) {
        opts = Object.assign({}, opts, {passive: false});
      } else {
        opts = {passive: false, capture: !!opts};
      }
    }
    return orig.call(this, type, fn, opts);
  };
})();
