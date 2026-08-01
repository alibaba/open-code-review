import React from 'react';
import BenchmarkSection from '../components/BenchmarkSection';
import Footer from '../components/Footer';
import FadeInSection from '../components/FadeInSection';

const BenchmarkPage: React.FC = () => {
  return (
    <div style={{ paddingTop: 72 }}>
      <FadeInSection>
        <BenchmarkSection />
      </FadeInSection>
      <FadeInSection>
        <Footer />
      </FadeInSection>
    </div>
  );
};

export default BenchmarkPage;
