export function ShellStyles() {
  return (
    <style>{`
      @keyframes pulse {
        0% { box-shadow: 0 0 0 0 rgba(16,185,129,0.4); }
        70% { box-shadow: 0 0 0 6px rgba(16,185,129,0); }
        100% { box-shadow: 0 0 0 0 rgba(16,185,129,0); }
      }
      @keyframes slideUp {
        from { transform: translateY(100%); }
        to { transform: translateY(0); }
      }

      /* Mobile layout */
      @media (max-width: 767px) {
        .desktop-nav { display: none !important; }
        .desktop-user-menu { display: none !important; }
        .desktop-live-indicator { display: none !important; }
        .bottom-tab-bar { display: flex !important; }
        .shell-main { padding-bottom: calc(70px + env(safe-area-inset-bottom)) !important; }
      }

      /* Desktop layout */
      @media (min-width: 768px) {
        .desktop-nav { display: flex !important; }
        .desktop-user-menu { display: flex !important; }
        .bottom-tab-bar { display: none !important; }
      }
    `}</style>
  )
}
