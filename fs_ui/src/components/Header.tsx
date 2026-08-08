import React from 'react';
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

  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 4, fontFamily: theme.fontHeading, fontWeight: 600, fontSize: 19, flexWrap: 'wrap' }}>
      {crumbs.map((crumb, i) => {
        const isLast = i === crumbs.length - 1;
        return (
          <React.Fragment key={crumb.path}>
            <span
              onClick={isLast ? undefined : () => onNavigate(crumb.path)}
              style={{ cursor: isLast ? 'default' : 'pointer', color: isLast ? theme.text : 'rgba(29,31,32,0.5)' }}
            >
              {crumb.label}
            </span>
            {!isLast && i > 0 && (
              <span style={{ margin: '0 2px', color: 'rgba(29,31,32,0.5)' }}>/</span>
            )}
          </React.Fragment>
        );
      })}
    </div>
  );
};

export default Header;
