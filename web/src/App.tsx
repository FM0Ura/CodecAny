import { Route, Routes } from "react-router-dom";
import { AppLayout } from "./components/AppLayout";
import { Dashboard } from "./routes/Dashboard";
import { Queue } from "./routes/Queue";
import { Approval } from "./routes/Approval";
import { Directories } from "./routes/Directories";
import { Placeholder } from "./routes/Placeholder";
import { DashboardSummaryProvider } from "./context/DashboardSummaryContext";

export default function App() {
  return (
    <DashboardSummaryProvider>
      <Routes>
        <Route element={<AppLayout />}>
          <Route index element={<Dashboard />} />
          <Route path="fila" element={<Queue />} />
          <Route path="aprovacao" element={<Approval />} />
          <Route path="diretorios" element={<Directories />} />
          <Route path="regras" element={<Placeholder screen="Regras" phase="Fase D" />} />
          <Route
            path="health-check"
            element={<Placeholder screen="Health Check" phase="Fase E" />}
          />
          <Route path="historico" element={<Placeholder screen="Histórico" phase="Fase E" />} />
          <Route path="*" element={<Placeholder screen="Página não encontrada" phase="—" />} />
        </Route>
      </Routes>
    </DashboardSummaryProvider>
  );
}
