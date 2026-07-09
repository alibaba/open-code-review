import React from 'react';
import QuickStartSection from '../components/QuickStartSection';
import Footer from '../components/Footer';
import FadeInSection from '../components/FadeInSection';

const QuickStartPage: React.FC = () => {
  return (
    <div style={{ paddingTop: 72 }}>
      <FadeInSection>
        <QuickStartSection />
      </FadeInSection>
      <FadeInSection>
        <Footer />
      </FadeInSection>
    </div>
  );
};

export default QuickStartPage;
