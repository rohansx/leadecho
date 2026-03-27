import { useState, useEffect } from 'react';
import { motion, useScroll, useMotionValueEvent } from 'framer-motion';
import { Radar } from 'lucide-react';

function GithubIcon({ size = 16 }: { size?: number }) {
  return (
    <svg viewBox="0 0 24 24" width={size} height={size} fill="currentColor">
      <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z"/>
    </svg>
  );
}
import ThemeToggle from './ThemeToggle';

const navLinks = [
  { label: 'Features', href: '/#features' },
  { label: 'Pricing', href: '/#pricing' },
  { label: 'Docs', href: '/docs' },
];

export default function FloatingNav() {
  const { scrollY } = useScroll();
  const [visible, setVisible] = useState(true);
  const [scrolled, setScrolled] = useState(false);
  const [mobileOpen, setMobileOpen] = useState(false);

  useMotionValueEvent(scrollY, 'change', (current) => {
    const prev = scrollY.getPrevious() ?? 0;
    const direction = current - prev;

    if (current < 50) {
      setVisible(true);
      setScrolled(false);
    } else {
      setScrolled(true);
      setVisible(direction < 0 || current < 200);
    }
  });

  useEffect(() => {
    const handler = () => { if (window.innerWidth > 768) setMobileOpen(false); };
    window.addEventListener('resize', handler);
    return () => window.removeEventListener('resize', handler);
  }, []);

  return (
    <>
      <motion.nav
        className={`floating-nav${scrolled ? ' floating-nav--glass' : ''}`}
        initial={{ y: 0, opacity: 1 }}
        animate={{
          y: visible ? 0 : -100,
          opacity: visible ? 1 : 0,
        }}
        transition={{ duration: 0.25, ease: 'easeInOut' }}
      >
        <div className="floating-nav-inner">
          <a className="nav-logo" href="/">
            <Radar size={20} strokeWidth={2.5} style={{ color: 'var(--accent)' }} />
            Lead<span>Echo</span>
          </a>

          <div className="floating-nav-links">
            {navLinks.map((link) => (
              <a key={link.label} href={link.href} className="floating-nav-link">
                {link.label}
              </a>
            ))}
            <a
              href="https://github.com/rohansx/leadecho"
              target="_blank"
              rel="noreferrer"
              className="floating-nav-link"
              style={{ display: 'inline-flex', alignItems: 'center', gap: 5 }}
            >
              <GithubIcon size={15} />
              GitHub
            </a>
          </div>

          <div className="floating-nav-right">
            <ThemeToggle />
            <a className="btn btn-outline btn-sm" href="/app/login">Sign in</a>
            <a className="btn btn-primary btn-sm" href="/app/register">Sign up</a>
            <button
              className={`hamburger${mobileOpen ? ' open' : ''}`}
              aria-label="Menu"
              onClick={() => setMobileOpen(!mobileOpen)}
            >
              <span /><span /><span />
            </button>
          </div>
        </div>
      </motion.nav>

      {mobileOpen && (
        <motion.div
          className="mobile-nav-overlay"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          onClick={() => setMobileOpen(false)}
        >
          <motion.div
            className="mobile-nav-menu"
            initial={{ y: -20, opacity: 0 }}
            animate={{ y: 0, opacity: 1 }}
            transition={{ duration: 0.2, delay: 0.05 }}
            onClick={(e) => e.stopPropagation()}
          >
            {navLinks.map((link) => (
              <a
                key={link.label}
                href={link.href}
                className="mobile-nav-link"
                onClick={() => setMobileOpen(false)}
              >
                {link.label}
              </a>
            ))}
            <a
              href="https://github.com/rohansx/leadecho"
              target="_blank"
              rel="noreferrer"
              className="mobile-nav-link"
              onClick={() => setMobileOpen(false)}
              style={{ display: 'flex', alignItems: 'center', gap: 8 }}
            >
              <GithubIcon size={16} />
              GitHub
            </a>
            <div className="mobile-nav-ctas">
              <a className="btn btn-outline" href="/app/login">Sign in</a>
              <a className="btn btn-primary" href="/app/register">Sign up</a>
            </div>
          </motion.div>
        </motion.div>
      )}
    </>
  );
}
