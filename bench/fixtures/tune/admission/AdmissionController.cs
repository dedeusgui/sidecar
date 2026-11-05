namespace Terminal.Admission
{
    public class AdmissionController
    {
        public const int MaxQueueLength = 50;

        public void RegisterEntry(string plate)
        {
            queue.Add(plate);
        }
    }
}
