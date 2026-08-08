import React, { useState } from 'react';
import { theme } from '../theme';

interface HeaderProps {
  currentPath: string;
  onNavigate: (path: string) => void;
}

// Breadcrumb: "/ segment / segment", slash-separated so it reads like a real path.
// The first crumb is labelled "/" rather than "root" so it can't be read as /root;
// it also stands in for the separator, so no slash is drawn immediately after it.
const Header: React.FC<HeaderProps> = ({ currentPath, onNavigate }) => {
  const segments = currentPath.split('/').filter(Boolean);
  const crumbs = [
    { label: '/', path: '/' },
    ...segments.map((seg, i) => ({ label: seg, path: '/' + segments.slice(0, i + 1).join('/') })),
  ];

  // Hover state is tracked rather than done in CSS because these are inline
  // styles; the last crumb is the current directory and stays inert.
  const [hovered, setHovered] = useState<number | null>(null);

  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 4, fontFamily: theme.fontHeading, fontWeight: 600, fontSize: 19, flexWrap: 'wrap' }}>
      {crumbs.map((crumb, i) => {
        const isLast = i === crumbs.length - 1;
        const isHovered = !isLast && hovered === i;
        return (
          <React.Fragment key={crumb.path}>
            <span
              onClick={isLast ? undefined : () => onNavigate(crumb.path)}
              onMouseEnter={isLast ? undefined : () => setHovered(i)}
              onMouseLeave={isLast ? undefined : () => setHovered(null)}
              title={isLast ? undefined : `Go to ${crumb.path}`}
              style={{
                cursor: isLast ? 'default' : 'pointer',
                color: isLast || isHovered ? theme.text : theme.textMuted,
                background: isHovered ? theme.accent100 : 'transparent',
                textDecoration: isHovered ? 'underline' : 'none',
                textUnderlineOffset: 3,
                padding: '2px 5px',
                margin: '-2px -5px',   // keep the hover pad from shifting the layout
                transition: 'color 0.1s, background 0.1s',
              }}
            >
              {crumb.label}
            </span>
            {!isLast && i > 0 && (
              <span style={{ margin: '0 2px', color: theme.textMuted }}>/</span>
            )}
          </React.Fragment>
        );
      })}
    </div>
  );
};

export default Header;
