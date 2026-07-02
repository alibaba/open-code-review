import React from 'react';
import Navbar from './Navbar';

interface LandingPageProps {
  children: React.ReactNode;
}

const LandingPage: React.FC<LandingPageProps> = ({ children }) => {
  return (
    <div
      style={{
        width: '100%',
        background: '#000000',
        overflowX: 'hidden',
      }}
    >
      <div
        style={{
          width: '100%',
          maxWidth: 1440,
          margin: '0 auto',
          display: 'flex',
          flexDirection: 'column',
        }}
      >
        <Navbar />
        {children}
      </div>
    </div>
  );
};

export default LandingPage;
